// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 字符/字符串操作

package zc

import (
	"bytes"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"unicode"
)

func EqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if ToLowerB(s[i]) != ToLowerB(t[i]) {
			return false
		}
	}
	return true
}

func HasPrefixFold(s, t string) bool {
	if len(s) < len(t) {
		return false
	}
	for i := 0; i < len(t); i++ {
		if ToLowerB(s[i]) != ToLowerB(t[i]) {
			return false
		}
	}
	return true
}

func ToLowerB(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// 首字母大写转小写
func LowerFirst(s string) string {
	if s == "" {
		return s
	}
	// r, size := utf8.DecodeRuneInString(s)
	// return string(unicode.ToLower(r)) + s[size:]
	return string(unicode.ToLower(rune(s[0]))) + s[1:]
}

// 驼峰转下划线
func Camel2Case(s string) string {
	if s == "" {
		return s
	}
	buf := bytes.NewBuffer([]byte{})
	for i, r := range s {
		if i == 0 {
			buf.WriteRune(unicode.ToLower(r))
			continue
		}
		if unicode.IsUpper(r) {
			buf.WriteRune('_')
			buf.WriteRune(unicode.ToLower(r))
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// ToStr ...
func ToStr(aa any) string {
	if bts, err := json.Marshal(aa); err != nil {
		return "<json marshal error>: " + err.Error()
	} else {
		return string(bts)
	}
}

// ToStr2 ...
func ToStr2(aa any) string {
	if bts, err := json.MarshalIndent(aa, "", "  "); err != nil {
		return "<json marshal error>: " + err.Error()
	} else {
		return string(bts)
	}
}

// 随机生成字符串， 0~f, 首字母不是 bb
// @param bb 首字母
func GenStr(bb string, ll int) string {
	str := []byte("0123456789abcdef")
	buf := make([]byte, ll-len(bb))
	for i := range buf {
		buf[i] = str[mrand.Intn(len(str))]
	}
	return bb + string(buf)
}

// 生成UUIDv4
func GenUUIDv4() (string, error) {
	// 1. 生成16个随机字节
	uuid := make([]byte, 16)
	if _, err := crand.Read(uuid); err != nil {
		return "", err // 随机数生成失败
	}

	// 2. 设置UUID版本和变体
	uuid[6] = (uuid[6] & 0x0F) | 0x40 // 第13位：0100（V4）
	uuid[8] = (uuid[8] & 0x3F) | 0x80 // 第17位：10xx（变体规范）

	// 3. 格式化为UUID字符串（8-4-4-4-12）
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

func UnicodeToRunes(srs ...[]byte) ([]byte, error) {
	var rst bytes.Buffer
	for _, src := range srs {
		n := len(src)
		for i := 0; i < n; {
			// 匹配\u转义序列：检查是否有足够字节，且当前为'\\'、下一个为'u'
			if i+1 < n && src[i] == '\\' && src[i+1] == '\\' {
				// 转义序列
				rst.WriteByte(src[i])
				rst.WriteByte(src[i+1])
				i += 2
			} else if i+5 <= n && src[i] == '\\' && src[i+1] == 'u' {
				// 提取4位十六进制字节（i+2到i+5）
				bts := src[i+2 : i+6]
				// 手动解析十六进制为Unicode码点（uint16范围：0~65535）
				// code, size := utf8.DecodeRune(bts)
				// if size == 0 {
				// 	return nil, fmt.Errorf("invalid unicode, \\u%s", string(bts))
				// }
				var code rune
				for _, b := range bts {
					code = code << 4
					if b >= '0' && b <= '9' {
						code += rune(b - '0')
					} else if b >= 'a' && b <= 'f' {
						code += rune(b - 'a' + 10)
					} else if b >= 'A' && b <= 'F' {
						code += rune(b - 'A' + 10)
					} else {
						return nil, fmt.Errorf("invalid unicode, \\u%s", string(bts))
					}
				}
				// 将码点转为UTF-8字节并写入结果（rune自动转UTF-8）
				rst.WriteRune(code)
				// 跳过已处理的6个字节（\uXXXX）
				i += 6
			} else {
				// 普通字节直接写入结果
				rst.WriteByte(src[i])
				i++
			}
		}
	}
	return rst.Bytes(), nil
}
