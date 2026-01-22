// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 通过环境遍历初始化配置参数
// 通过TAG标签处理格式化参数

package zc

import (
	"os"
	"reflect"
	"strconv"
	"strings"
)

// ENV 解析结果的根结构
type ENV struct {
	Prefix string
}

// 新建 ENV 解析器
func NewENV(prefix string) *ENV {
	return &ENV{Prefix: prefix}
}

// 解析 ENV 数据
func (aa *ENV) Load(val any) error {
	return aa.Decode(val, CFG_TAG)
}

// 解析 ENV 数据
func (aa *ENV) Decode(val any, tag string) error {
	tags := ToTagVal(val, tag)
	for _, tag := range tags {
		// log.Println(tag.Keys)
		vkey := strings.ToUpper(aa.Prefix + "_" + strings.Join(tag.Keys, "_"))
		venv := os.Getenv(vkey)
		if val, err := StrToBV(tag.Field.Type, venv); err == nil {
			tag.Value.Set(reflect.ValueOf(val))
		} else if vkey[len(vkey)-1] == 'S' {
			idx := -1
			arr := []string{}
			for {
				idx++
				if venv = os.Getenv(vkey + "_" + strconv.Itoa(idx)); venv == "" {
					break
				}
				arr = append(arr, venv)
			}
			if len(arr) > 0 {
				if tag.Field.Type.Kind() == reflect.Map && //
					tag.Field.Type.Key().Kind() == reflect.String && //
					tag.Field.Type.Elem().Kind() == reflect.String {
					// tag.Field.Type = map[string]string
					vvv := map[string]string{}
					for _, vv := range arr {
						vv = strings.TrimSpace(vv)
						if vv == "" {
							continue
						}
						kv := strings.SplitN(vv, "=", 2)
						if len(kv) == 2 {
							vvv[kv[0]] = kv[1]
						} else {
							vvv[kv[0]] = ""
						}
					}
					tag.Value.Set(reflect.ValueOf(vvv))

					// } else if tag.Field.Type.Kind() == reflect.Slice && //
					// 	tag.Field.Type.Elem().Len() == 2 && //
					// 	tag.Field.Type.Elem().Elem().Kind() == reflect.String {
					// 	// tag.Field.Type = [][2]string
					// 	vvv := [][2]string{}
					// 	for _, vv := range arr {
					// 		vv = strings.TrimSpace(vv)
					// 		if vv == "" {
					// 			continue
					// 		}
					// 		kv := strings.SplitN(vv, "=", 2)
					// 		if len(kv) == 2 {
					// 			vvv = append(vvv, [2]string{kv[0], kv[1]})
					// 		} else {
					// 			vvv = append(vvv, [2]string{vv, ""})
					// 		}
					// 	}
					// 	tag.Value.Set(reflect.ValueOf(vvv))

				} else if val, err := ToBasicValue(tag.Field.Type, arr); err == nil {
					tag.Value.Set(reflect.ValueOf(val))
				}
			}
		}
	}
	return nil
}

// ----------------------------------------------------

// TAG 解析结果的根结构
type TAG struct {
}

// 新建 TAG 解析器
func NewTAG() *TAG {
	return &TAG{}
}

// 解析 TAG 数据
func (aa *TAG) Load(val any) error {
	return aa.Decode(val, CFG_TAG)
}

// 解析 TAG 数据
func (aa *TAG) Decode(val any, tag string) error {
	tags := ToTagVal(val, tag)
	for _, tag := range tags {
		if tag.Value.IsValid() {
			continue
		}
		// 使用默认值进行初始化
		vtag := tag.Field.Tag.Get("default")
		if val, err := StrToBV(tag.Field.Type, vtag); err == nil {
			tag.Value.Set(reflect.ValueOf(val))
		}
	}
	return nil
}
