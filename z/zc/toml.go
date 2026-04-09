// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 一个基础 toml 解析器

package zc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TOML解析结果的根结构
type TOML struct {
	data map[string]any
	err  error
}

// 新建TOML解析器
func NewTOML(bts []byte) *TOML {
	toml := &TOML{data: make(map[string]any)}
	if bts != nil {
		toml.err = ParseTOML(bts, toml.data)
	}
	return toml
}

// 解析TOML数据
func (aa *TOML) Load(val any) error {
	return aa.Decode(val, CFG_TAG)
}

// 获取解析结果
func (aa *TOML) Map() map[string]any {
	return aa.data
}

// 解析TOML数据
func (aa *TOML) Decode(val any, tag string) error {
	if aa.err != nil {
		return aa.err
	}
	_, err := MapToStruct(val, aa.data, tag)
	return err
}

// ---------------------------------------------------------------------

// 解析TOML文件
func ParseTOML(bts []byte, rmap map[string]any) error {
	stmts, err := splitTOMLStatements(string(bts))
	if err != nil {
		return err
	}

	current := []string{}
	for _, stmt := range stmts {
		line := strings.TrimSpace(stmt)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[[") {
			if !strings.HasSuffix(line, "]]") {
				return fmt.Errorf("invalid array table header: %s", line)
			}
			parts, err := parseTOMLDottedKey(strings.TrimSpace(line[2 : len(line)-2]))
			if err != nil {
				return err
			}
			if err := newTOMLNestedValue(parts, true, rmap); err != nil {
				return err
			}
			current = parts
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") || strings.HasPrefix(line, "[[") {
				return fmt.Errorf("invalid table header: %s", line)
			}
			parts, err := parseTOMLDottedKey(strings.TrimSpace(line[1 : len(line)-1]))
			if err != nil {
				return err
			}
			if err := newTOMLNestedValue(parts, false, rmap); err != nil {
				return err
			}
			current = parts
			continue
		}

		idx, err := indexTOMLTopLevelByte(line, '=')
		if err != nil {
			return err
		}
		if idx < 0 {
			return fmt.Errorf("invalid key/value expression: %s", line)
		}

		keys, err := parseTOMLDottedKey(strings.TrimSpace(line[:idx]))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return fmt.Errorf("empty key in expression: %s", line)
		}

		val, err := parseTOMLValue(strings.TrimSpace(line[idx+1:]))
		if err != nil {
			return err
		}
		if err := setTOMLNestedValue(current, keys, val, rmap); err != nil {
			return err
		}
	}

	return nil
}

// 设置嵌套结构的值，路径由当前表头和键组成
func setTOMLNestedValue(path []string, keys []string, val any, rmap map[string]any) error {
	fullPath := append(append([]string{}, path...), keys...)
	return assignTOMLMapValue(rmap, fullPath, val)
}

// 创建一个新的嵌套结构，路径由表头组成，isa表示是否为数组表
func newTOMLNestedValue(path []string, isa bool, rmap map[string]any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty table path")
	}
	curr := rmap
	lenx := len(path)
	for _, part := range path[:lenx-1] {
		if data, ok := curr[part].(map[string]any); ok {
			curr = data
		} else if data, ok := curr[part].([]map[string]any); ok {
			if len(data) == 0 {
				return fmt.Errorf("array table %q has no active item", part)
			}
			curr = data[len(data)-1]
		} else if curr[part] == nil {
			next := make(map[string]any)
			curr[part] = next
			curr = next
		} else {
			return fmt.Errorf("path conflict at %q", part)
		}
	}
	last := path[lenx-1]
	if !isa {
		if curr[last] == nil {
			curr[last] = make(map[string]any)
			return nil
		}
		if _, ok := curr[last].(map[string]any); ok {
			return nil
		}
		return fmt.Errorf("table %q conflicts with existing value", strings.Join(path, "."))
	} else if data, ok := curr[path[lenx-1]].([]map[string]any); ok {
		curr[path[lenx-1]] = append(data, make(map[string]any))
		return nil
	} else if curr[path[lenx-1]] == nil {
		curr[path[lenx-1]] = []map[string]any{make(map[string]any)}
		return nil
	} else {
		return fmt.Errorf("array table %q conflicts with existing value", strings.Join(path, "."))
	}
}

// 解析TOML语句，返回每条语句的字符串切片
func splitTOMLStatements(src string) ([]string, error) {
	statements := []string{}
	var buf strings.Builder

	var quote rune
	var multiline bool
	escaped := false
	bracketDepth := 0
	braceDepth := 0

	for idx := 0; idx < len(src); {
		r, size := utf8.DecodeRuneInString(src[idx:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("invalid utf-8 input")
		}

		if quote != 0 {
			buf.WriteRune(r)
			if quote == '"' {
				if multiline {
					if r == '"' && hasTOMLRepeatedRune(src[idx:], '"', 3) {
						buf.WriteString(src[idx+size : idx+size*3])
						idx += size * 3
						quote = 0
						multiline = false
						escaped = false
						continue
					}
					if r == '\\' && !escaped {
						escaped = true
					} else {
						escaped = false
					}
				} else {
					if r == '"' && !escaped {
						quote = 0
					} else if r == '\\' && !escaped {
						escaped = true
					} else {
						escaped = false
					}
				}
			} else if multiline {
				if r == '\'' && hasTOMLRepeatedRune(src[idx:], '\'', 3) {
					buf.WriteString(src[idx+size : idx+size*3])
					idx += size * 3
					quote = 0
					multiline = false
					escaped = false
					continue
				}
			} else if r == '\'' {
				quote = 0
			}
			idx += size
			continue
		}

		switch r {
		case '#':
			for idx < len(src) {
				r2, s2 := utf8.DecodeRuneInString(src[idx:])
				if r2 == '\n' {
					break
				}
				idx += s2
			}
			continue
		case '"', '\'':
			buf.WriteRune(r)
			if hasTOMLRepeatedRune(src[idx:], r, 3) {
				buf.WriteString(src[idx+size : idx+size*3])
				idx += size * 3
				quote = r
				multiline = true
				escaped = false
				continue
			}
			quote = r
			multiline = false
			escaped = false
		case '[':
			bracketDepth++
			buf.WriteRune(r)
		case ']':
			bracketDepth--
			if bracketDepth < 0 {
				return nil, fmt.Errorf("unexpected ]")
			}
			buf.WriteRune(r)
		case '{':
			braceDepth++
			buf.WriteRune(r)
		case '}':
			braceDepth--
			if braceDepth < 0 {
				return nil, fmt.Errorf("unexpected }")
			}
			buf.WriteRune(r)
		case '\n':
			if bracketDepth == 0 && braceDepth == 0 {
				stmt := strings.TrimSpace(buf.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				buf.Reset()
			} else {
				buf.WriteRune(r)
			}
		default:
			buf.WriteRune(r)
		}
		idx += size
	}

	if quote != 0 {
		return nil, fmt.Errorf("unterminated string")
	}
	if bracketDepth != 0 || braceDepth != 0 {
		return nil, fmt.Errorf("unterminated composite value")
	}
	if stmt := strings.TrimSpace(buf.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements, nil
}

// 判断字符串开头是否有连续count个target字符
func hasTOMLRepeatedRune(src string, target rune, count int) bool {
	idx := 0
	for i := 0; i < count; i++ {
		r, size := utf8.DecodeRuneInString(src[idx:])
		if r != target {
			return false
		}
		idx += size
	}
	return true
}

// 在TOML语句中查找顶层的目标字符，忽略引号内和嵌套结构内的字符
func indexTOMLTopLevelByte(src string, target byte) (int, error) {
	var quote rune
	bracketDepth := 0
	braceDepth := 0
	escaped := false

	for idx := 0; idx < len(src); idx++ {
		ch := src[idx]
		if quote != 0 {
			if quote == '"' {
				if ch == '"' && !escaped {
					quote = 0
				} else if ch == '\\' && !escaped {
					escaped = true
					continue
				}
				escaped = false
			} else if ch == '\'' {
				quote = 0
			}
			continue
		}

		switch ch {
		case '"', '\'':
			quote = rune(ch)
			escaped = false
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		default:
			if ch == target && bracketDepth == 0 && braceDepth == 0 {
				return idx, nil
			}
		}
		if bracketDepth < 0 || braceDepth < 0 {
			return -1, fmt.Errorf("invalid nesting in %q", src)
		}
	}
	if quote != 0 {
		return -1, fmt.Errorf("unterminated quoted content in %q", src)
	}
	return -1, nil
}

// 解析TOML键，支持点分隔和引号包裹的键
func parseTOMLDottedKey(expr string) ([]string, error) {
	parts := []string{}
	var buf strings.Builder
	var quote rune
	escaped := false

	flush := func() error {
		part := strings.TrimSpace(buf.String())
		buf.Reset()
		if part == "" {
			return fmt.Errorf("invalid dotted key: %s", expr)
		}
		if (strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`)) ||
			(strings.HasPrefix(part, `'`) && strings.HasSuffix(part, `'`)) {
			parsed, err := parseTOMLString(part)
			if err != nil {
				return err
			}
			parts = append(parts, parsed)
			return nil
		}
		parts = append(parts, part)
		return nil
	}

	for idx := 0; idx < len(expr); idx++ {
		ch := expr[idx]
		if quote != 0 {
			buf.WriteByte(ch)
			if quote == '"' {
				if ch == '"' && !escaped {
					quote = 0
				} else if ch == '\\' && !escaped {
					escaped = true
					continue
				}
				escaped = false
			} else if ch == '\'' {
				quote = 0
			}
			continue
		}

		switch ch {
		case '"', '\'':
			quote = rune(ch)
			escaped = false
			buf.WriteByte(ch)
		case '.':
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			buf.WriteByte(ch)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted key: %s", expr)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return parts, nil
}

// 解析TOML值，支持字符串、数组和内联表
func parseTOMLValue(expr string) (any, error) {
	val := strings.TrimSpace(expr)
	if val == "" {
		return nil, fmt.Errorf("empty value")
	}
	if strings.HasPrefix(val, "{") {
		return parseTOMLInlineTable(val)
	}
	if strings.HasPrefix(val, "[") {
		return parseTOMLArray(val)
	}
	if strings.HasPrefix(val, `"`) || strings.HasPrefix(val, `'`) {
		return parseTOMLString(val)
	}
	return val, nil
}

// 解析TOML数组，返回一个切片，元素可以是字符串、内联表或混合类型
func parseTOMLArray(expr string) (any, error) {
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil, fmt.Errorf("invalid array value: %s", expr)
	}
	body := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if body == "" {
		return []string{}, nil
	}
	items, err := splitTOMLTopLevelItems(body, ',')
	if err != nil {
		return nil, err
	}

	values := make([]any, 0, len(items))
	allScalar := true
	allMaps := true
	for _, item := range items {
		parsed, err := parseTOMLValue(item)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
		if _, ok := parsed.(string); !ok {
			allScalar = false
		}
		if _, ok := parsed.(map[string]any); !ok {
			allMaps = false
		}
	}

	if allScalar {
		result := make([]string, len(values))
		for idx, item := range values {
			result[idx] = item.(string)
		}
		return result, nil
	}
	if allMaps {
		result := make([]map[string]any, len(values))
		for idx, item := range values {
			result[idx] = item.(map[string]any)
		}
		return result, nil
	}
	return values, nil
}

// 解析TOML内联表，返回一个map[string]any，支持点分隔的键
func parseTOMLInlineTable(expr string) (map[string]any, error) {
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, fmt.Errorf("invalid inline table: %s", expr)
	}
	body := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	out := map[string]any{}
	if body == "" {
		return out, nil
	}
	items, err := splitTOMLTopLevelItems(body, ',')
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		idx, err := indexTOMLTopLevelByte(item, '=')
		if err != nil {
			return nil, err
		}
		if idx < 0 {
			return nil, fmt.Errorf("invalid inline table item: %s", item)
		}
		keys, err := parseTOMLDottedKey(strings.TrimSpace(item[:idx]))
		if err != nil {
			return nil, err
		}
		val, err := parseTOMLValue(strings.TrimSpace(item[idx+1:]))
		if err != nil {
			return nil, err
		}
		if err := assignTOMLMapValue(out, keys, val); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// 解析TOML字符串，支持基本字符串、字面字符串和多行字符串
func splitTOMLTopLevelItems(src string, sep byte) ([]string, error) {
	items := []string{}
	var buf strings.Builder
	var quote rune
	escaped := false
	bracketDepth := 0
	braceDepth := 0

	flush := func() {
		item := strings.TrimSpace(buf.String())
		buf.Reset()
		if item != "" {
			items = append(items, item)
		}
	}

	for idx := 0; idx < len(src); idx++ {
		ch := src[idx]
		if quote != 0 {
			buf.WriteByte(ch)
			if quote == '"' {
				if ch == '"' && !escaped {
					quote = 0
				} else if ch == '\\' && !escaped {
					escaped = true
					continue
				}
				escaped = false
			} else if ch == '\'' {
				quote = 0
			}
			continue
		}

		switch ch {
		case '"', '\'':
			quote = rune(ch)
			escaped = false
			buf.WriteByte(ch)
		case '[':
			bracketDepth++
			buf.WriteByte(ch)
		case ']':
			bracketDepth--
			if bracketDepth < 0 {
				return nil, fmt.Errorf("invalid array nesting: %s", src)
			}
			buf.WriteByte(ch)
		case '{':
			braceDepth++
			buf.WriteByte(ch)
		case '}':
			braceDepth--
			if braceDepth < 0 {
				return nil, fmt.Errorf("invalid inline table nesting: %s", src)
			}
			buf.WriteByte(ch)
		default:
			if ch == sep && bracketDepth == 0 && braceDepth == 0 {
				flush()
				continue
			}
			buf.WriteByte(ch)
		}
	}
	if quote != 0 || bracketDepth != 0 || braceDepth != 0 {
		return nil, fmt.Errorf("unterminated composite item: %s", src)
	}
	flush()
	return items, nil
}

// 将值赋给TOML内联表，支持点分隔的键，自动创建嵌套结构
func assignTOMLMapValue(dst map[string]any, path []string, val any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty key path")
	}
	curr := dst
	for _, key := range path[:len(path)-1] {
		next, ok := curr[key]
		if !ok {
			child := make(map[string]any)
			curr[key] = child
			curr = child
			continue
		}
		switch data := next.(type) {
		case map[string]any:
			curr = data
		case []map[string]any:
			if len(data) == 0 {
				return fmt.Errorf("array table %q has no active item", key)
			}
			curr = data[len(data)-1]
		default:
			return fmt.Errorf("key path conflict at %q", key)
		}
	}
	last := path[len(path)-1]
	if _, exists := curr[last]; exists {
		return fmt.Errorf("duplicate key %q", strings.Join(path, "."))
	}
	curr[last] = val
	return nil
}

// 判断字符串是否全由TOML允许的空白字符或换行符组成
func parseTOMLString(expr string) (string, error) {
	if strings.HasPrefix(expr, `"""`) {
		if !strings.HasSuffix(expr, `"""`) || len(expr) < 6 {
			return "", fmt.Errorf("invalid multiline basic string")
		}
		content := strings.TrimPrefix(expr[3:len(expr)-3], "\n")
		return unescapeTOMLBasicString(content, true)
	}
	if strings.HasPrefix(expr, `'''`) {
		if !strings.HasSuffix(expr, `'''`) || len(expr) < 6 {
			return "", fmt.Errorf("invalid multiline literal string")
		}
		content := strings.TrimPrefix(expr[3:len(expr)-3], "\n")
		return content, nil
	}
	if strings.HasPrefix(expr, `"`) {
		return strconv.Unquote(expr)
	}
	if strings.HasPrefix(expr, `'`) {
		if len(expr) < 2 || !strings.HasSuffix(expr, `'`) {
			return "", fmt.Errorf("invalid literal string")
		}
		return expr[1 : len(expr)-1], nil
	}
	return "", fmt.Errorf("invalid string value: %s", expr)
}

// 解析TOML基本字符串，处理转义序列和多行字符串中的续行
func unescapeTOMLBasicString(src string, multiline bool) (string, error) {
	var out strings.Builder
	for idx := 0; idx < len(src); {
		r, size := utf8.DecodeRuneInString(src[idx:])
		if r != '\\' {
			out.WriteRune(r)
			idx += size
			continue
		}

		idx += size
		if idx >= len(src) {
			return "", fmt.Errorf("unfinished escape sequence")
		}
		r, size = utf8.DecodeRuneInString(src[idx:])
		if multiline && isTOMLWhitespaceOrNewline(r) {
			for idx < len(src) {
				r, size = utf8.DecodeRuneInString(src[idx:])
				if !isTOMLWhitespaceOrNewline(r) {
					break
				}
				idx += size
			}
			continue
		}

		switch r {
		case 'b':
			out.WriteByte('\b')
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'f':
			out.WriteByte('\f')
		case 'r':
			out.WriteByte('\r')
		case '"':
			out.WriteByte('"')
		case '\\':
			out.WriteByte('\\')
		case 'u', 'U':
			digits := 4
			if r == 'U' {
				digits = 8
			}
			idx += size
			if idx+digits > len(src) {
				return "", fmt.Errorf("invalid unicode escape")
			}
			code, err := strconv.ParseUint(src[idx:idx+digits], 16, 32)
			if err != nil {
				return "", err
			}
			out.WriteRune(rune(code))
			idx += digits
			continue
		default:
			return "", fmt.Errorf("unsupported escape sequence: \\%c", r)
		}
		idx += size
	}
	return out.String(), nil
}

// 判断字符是否是TOML允许的空白字符或换行符，用于处理多行基本字符串中的续行
func isTOMLWhitespaceOrNewline(r rune) bool {
	return r == '\n' || r == '\r' || unicode.IsSpace(r)
}
