// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// json 序列化， 注意： 暂时没有反序列化处理的想法
/*
func (xxx XXX) MarshalJSON() ([]byte, error) {
	return zc.ToJsonBytes(&rc, "json", zc.LowerFirst, false)
}
*/

package zc

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
)

func ToJsonMap(val any, tag string, kfn func(string) string, non bool) (map[string]any, []string, error) {
	if tag == "" {
		tag = "json"
	}
	lst := []string{}
	rst := map[string]any{}
	vType := reflect.TypeOf(val)
	value := reflect.ValueOf(val)
	if vType.Kind() == reflect.Pointer {
		vType = vType.Elem()
		value = value.Elem()
	}
	for i := 0; i < vType.NumField(); i++ {
		if non && value.Field(i).IsZero() {
			continue
		}
		vField := vType.Field(i)
		vTag := vField.Tag.Get(tag)
		if vTag == "-" {
			continue
		}
		if vField.Anonymous && vField.Type.Kind() == reflect.Struct {
			// 匿名字段
			vvv, kkk, err := ToJsonMap(value.Field(i).Interface(), tag, kfn, non)
			if err != nil {
				return nil, nil, err
			}
			maps.Copy(rst, vvv)
			lst = append(lst, kkk...)
			continue
		}
		// 普通字段
		vName := vField.Name
		if vTag == "" && kfn != nil {
			vName = kfn(vName)
		} else if vTag != "" {
			if idx := strings.IndexRune(vTag, ','); idx > 0 {
				vName = vTag[:idx]
			} else {
				vName = vTag
			}
		}
		rst[vName] = value.Field(i).Interface()
		lst = append(lst, vName)
	}
	return rst, lst, nil
}

//	func (r Data) MarshalJSON() ([]byte, error) {
//		return cfg.ToJsonBytes(&r, "json", cfg.LowerFirst, false)
//	}
//
// 修改字段名
//
// - @param val 结构体
// - @param tag 标签
// - @param kfn 键名转换函数
// - @param non 是否忽略零值
func ToJsonBytes(val any, tag string, kfn func(string) string, non bool) ([]byte, error) {
	vvv, kkk, err := ToJsonMap(val, tag, kfn, non)
	if err != nil {
		return nil, err
	}
	// return json.Marshal(vvv)
	buf := bytes.NewBuffer([]byte{'{'})
	for _, key := range kkk {
		bts, err := json.Marshal(vvv[key])
		if err != nil {
			return nil, err
		}
		buf.WriteByte('"')
		buf.WriteString(key)
		buf.WriteByte('"')
		buf.WriteByte(':')
		buf.Write(bts)
		buf.WriteByte(',')
	}
	if buf.Len() > 1 {
		buf.Truncate(buf.Len() - 1)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
