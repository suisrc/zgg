// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 一个基础 toml 解析器

package cfg

import (
	"bufio"
	"bytes"
	"strings"
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
		toml.err = toml.Parse(bts)
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
func (aa *TOML) Parse(bts []byte) error {
	// os.ReadFile()
	scanner := bufio.NewScanner(bytes.NewReader(bts))
	current := []string{} // 记录当前嵌套路径，如["database", "mysql"]
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // 跳过空行和注释
		}

		// 匹配[table]或[table.subtable]
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			// tblname := strings.Trim(line, "[]")
			tblname := line[2 : len(line)-2]
			current = strings.Split(tblname, ".")
			aa.newNestedValue(current, true)
			continue
		} else if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// tblname := strings.Trim(line, "[]")
			tblname := line[1 : len(line)-1]
			current = strings.Split(tblname, ".")
			aa.newNestedValue(current, false)
			continue
		}

		// 匹配键值对（key = value）
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // 忽略无效行
		}
		key := strings.TrimSpace(parts[0])
		if len(key) > 2 && key[0] == '"' && key[len(key)-1] == '"' {
			key = key[1 : len(key)-1]
		}
		val := strings.TrimSpace(parts[1])

		// 将键值对存入对应嵌套结构
		aa.setNestedValue(current, key, ToStrOrArr(val))
	}

	return scanner.Err()
}

// 递归设置嵌套结构的值
func (aa *TOML) setNestedValue(path []string, key string, val any) {
	curr := aa.data
	for _, part := range path {
		if data, ok := curr[part].(map[string]any); ok {
			curr = data
		} else if data, ok := curr[part].([]map[string]any); ok {
			curr = data[len(data)-1]
		} else {
			return // 忽略无效路径
		}
	}
	curr[key] = val
}

// 递归设置嵌套结构的值
func (aa *TOML) newNestedValue(path []string, isa bool) {
	curr := aa.data
	lenx := len(path)
	for _, part := range path[:lenx-1] {
		if data, ok := curr[part].(map[string]any); ok {
			curr = data
		} else if data, ok := curr[part].([]map[string]any); ok {
			curr = data[len(data)-1]
		} else {
			curr[part] = make(map[string]any)
			curr = curr[part].(map[string]any)
		}
	}
	if !isa {
		curr[path[lenx-1]] = make(map[string]any)
	} else if data, ok := curr[path[lenx-1]].([]map[string]any); ok {
		curr[path[lenx-1]] = append(data, make(map[string]any))
	} else {
		curr[path[lenx-1]] = []map[string]any{make(map[string]any)}
	}
}
