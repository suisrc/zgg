// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// map - struct 相互转换

package zc

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// AsMap ...
func AsMap(aa any) map[string]any {
	ref := reflect.ValueOf(aa)
	if ref.Kind() != reflect.Map {
		return nil // panic("obj is not map")
	}
	rss := make(map[string]any)
	for _, key := range ref.MapKeys() {
		rss[key.String()] = ref.MapIndex(key).Interface()
	}
	return rss
}

// Map2ToStruct
func Map2ToStruct[T any](target T, source map[string][]string, tagkey string) (T, error) {
	tags, kind := ToTag(target, tagkey, false, nil)
	if kind != reflect.Struct {
		return target, errors.New("target type is not struct")
	}
	for _, tag := range tags {
		val := source[tag.Tags[0]]
		if val == nil {
			// 默认值
			if ttv := tag.Field.Tag.Get("default"); ttv != "" {
				val = ToStrArr(ttv)
			}
		}
		if val == nil {
			continue
		}
		if value, err := ToBasicValue(tag.Field.Type, val); err == nil {
			tag.Value.Set(reflect.ValueOf(value))
		}
	}
	return target, nil
}

// ToMap ... 注意， 转换时候，确定没有循环引用，否则会出现异常
func ToMap(target any, tagkey string, isdeep bool) map[string]any {
	tags, kind := ToTagMust(target, tagkey)
	if kind == reflect.Map {
		return AsMap(target)
	} else if kind != reflect.Struct {
		return nil // panic("obj is not struct")
	}
	if isdeep {
		data := make(map[string]any)
		for _, tag := range tags {
			fty := tag.Field.Type
			if fty.Kind() == reflect.Struct || //
				fty.Kind() == reflect.Pointer && fty.Elem().Kind() == reflect.Struct {
				// struct -> map | *struct -> map
				data[tag.Tags[0]] = ToMap(tag.Value.Interface(), tagkey, true)
			} else if fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Struct || //
				fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
				// []struct -> []map | []*struct -> []map
				smap := make([]map[string]any, 0)
				slen := tag.Value.Len()
				for i := range slen {
					smap = append(smap, ToMap(tag.Value.Index(i).Interface(), tagkey, true))
				}
				data[tag.Tags[0]] = smap
			} else if fty.Kind() == reflect.Map && fty.Elem().Kind() == reflect.Struct || //
				fty.Kind() == reflect.Map && fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
				// map[string]struct -> map[string]map
				smap := make(map[string]any)
				vmap := tag.Value.MapRange()
				for vmap.Next() {
					ekey := vmap.Key().Interface().(string)
					smap[ekey] = ToMap(vmap.Value().Interface(), tagkey, true)
				}
				data[tag.Tags[0]] = smap
			} else {
				// other -> map.key
				data[tag.Tags[0]] = tag.Value.Interface()
			}
		}
		return data
	} else {
		// 只处理一层，浅拷贝
		data := make(map[string]any)
		for _, tag := range tags {
			data[tag.Tags[0]] = tag.Value.Interface()
		}
		return data
	}
}

// --------------------------------------------------------------------------
// --------------------------------------------------------------------------
// --------------------------------------------------------------------------

// MapToStructOrMap ... 警告， 函数存在风险，谨慎使用，防止 source 存在循环引用的情况
func MapToStructOrMap[T any](target T, source map[string]any, tagkey string) (T, error) {
	if vtype := reflect.TypeOf(target); vtype.Kind() == reflect.Map {
		value := reflect.ValueOf(target)
		for kk, vv := range source {
			value.SetMapIndex(reflect.ValueOf(kk), reflect.ValueOf(vv))
		}
		return target, nil
	} else if vtype.Kind() != reflect.Struct {
		return target, errors.New("target type is not map or struct")
	}
	return MapToStruct(target, source, tagkey)
}

// MapToStruct ... 警告， 函数存在风险，谨慎使用，防止 source 存在循环引用的情况
func MapToStruct[T any](target T, source map[string]any, tagkey string) (result T, reserr error) {
	defer func() {
		if p := recover(); p != nil {
			reserr = fmt.Errorf("panic error: %v", p)
			// log.Println("MapToStruct error:", reserr)
		}
	}()
	result = target
	tags, kind := ToTagMust(target, tagkey)
	if kind != reflect.Struct {
		reserr = errors.New("target type is not struct")
		return
	}
	for _, tag := range tags {
		// 获取字段对应值
		val := source[tag.Tags[0]]
		if val == nil {
			// 没有值， 使用默认值
			if ttv := tag.Field.Tag.Get("default"); ttv != "" {
				val = ToStrOrArr(ttv)
			}
		}
		if val == nil {
			continue
		}
		// -----------------------------------------------------------------------------
		vty := reflect.TypeOf(val)
		fty := tag.Field.Type
		if vty == fty {
			// basic type -> field
			tag.Value.Set(reflect.ValueOf(val))
			// 类型相同，直接赋值
		} else if vty.Kind() == reflect.String {
			// string -> field
			if vvv, err := ToBasicValue(fty, []string{val.(string)}); err == nil {
				tag.Value.Set(reflect.ValueOf(vvv))
			} // 通过 ToBasicValue 获取基础类型值
		} else if vty.Kind() == reflect.Slice && vty.Elem().Kind() == reflect.String {
			// []string -> field
			if vvv, err := ToBasicValue(fty, val.([]string)); err == nil {
				tag.Value.Set(reflect.ValueOf(vvv))
			} // 通过 ToBasicValue 获取基础类型值
		} else if vty.Kind() == reflect.Map && fty.Kind() == reflect.Struct {
			// map[string]any -> struct
			MapToStruct(tag.Value.Addr().Interface(), val.(map[string]any), tagkey)
			// 使用函数自身递归赋值
		} else if vty.Kind() == reflect.Map && fty.Kind() == reflect.Pointer && fty.Elem().Kind() == reflect.Struct {
			// map[string]any -> *struct
			if tag.Value.IsNil() {
				tag.Value.Set(reflect.New(fty.Elem()).Elem().Addr())
			}
			MapToStruct(tag.Value.Interface(), val.(map[string]any), tagkey)
			// 使用函数自身递归赋值,指针
		} else if vty.Kind() == reflect.Slice && vty.Elem().Kind() == reflect.Map && //
			fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Struct {
			// []map[string]any -> []struct
			if vva, ok := val.([]map[string]any); ok {
				if tag.Value.IsNil() {
					tag.Value.Set(reflect.New(fty).Elem())
				}
				vdx := tag.Value.Len()
				for idx, vvc := range vva {
					if idx >= vdx {
						tag.Value.Set(reflect.Append(tag.Value, reflect.New(fty.Elem()).Elem()))
					}
					vvb := tag.Value.Index(idx).Addr()
					MapToStruct(vvb.Interface(), vvc, tagkey)
				}
			} // 切片， 需要便利赋值
		} else if vty.Kind() == reflect.Slice && vty.Elem().Kind() == reflect.Map && //
			fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
			// []map[string]any -> []*struct
			if vva, ok := val.([]map[string]any); ok {
				if tag.Value.IsNil() {
					tag.Value.Set(reflect.New(fty).Elem())
				}
				vdx := tag.Value.Len()
				for idx, vvc := range vva {
					if idx >= vdx {
						tag.Value.Set(reflect.Append(tag.Value, reflect.New(fty.Elem().Elem()).Elem().Addr()))
					}
					vvb := tag.Value.Index(idx)
					MapToStruct(vvb.Interface(), vvc, tagkey)
				}
			} // 切片， 需要便利赋值
		} else if vty.Kind() == reflect.Map && fty.Kind() == reflect.Map && //
			fty.Elem().Kind() == reflect.Struct {
			// map[string]any -> map[string]struct
			if vva, ok := val.(map[string]any); ok {
				if tag.Value.IsNil() {
					tag.Value.Set(reflect.MakeMap(fty))
				}
				for kk, vv := range vva {
					if vc, ok := vv.(map[string]any); ok {
						vkk := reflect.ValueOf(kk)
						vvb := tag.Value.MapIndex(vkk)
						if !vvb.IsValid() || vvb.IsNil() {
							vvb = reflect.New(fty.Elem()).Elem()
						}
						MapToStruct(vvb.Addr().Interface(), vc, tagkey)
						tag.Value.SetMapIndex(vkk, vvb)
					}
				}
			}
		} else if vty.Kind() == reflect.Map && fty.Kind() == reflect.Map && //
			fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
			// map[string]any -> map[string]*struct
			if vva, ok := val.(map[string]any); ok {
				if tag.Value.IsNil() {
					tag.Value.Set(reflect.MakeMap(fty))
				}
				for kk, vv := range vva {
					if vc, ok := vv.(map[string]any); ok {
						vkk := reflect.ValueOf(kk)
						vvb := tag.Value.MapIndex(vkk)
						if !vvb.IsValid() || vvb.IsNil() {
							vvb = reflect.New(fty.Elem().Elem()).Elem().Addr()
							tag.Value.SetMapIndex(vkk, vvb)
						}
						MapToStruct(vvb.Interface(), vc, tagkey)
					}
				}
			}
		} else if vty.Kind() == reflect.Map && fty.Kind() == reflect.Map && //
			fty.Elem().Kind() == reflect.String {
			// map[string]any -> map[string]string
			if vva, ok := val.(map[string]any); ok {
				if tag.Value.IsNil() {
					tag.Value.Set(reflect.MakeMap(fty))
				}
				for kk, vv := range vva {
					if vc, ok := vv.(string); ok && len(vc) > 0 {
						vkk := reflect.ValueOf(kk)
						vvv := reflect.ValueOf(vc)
						tag.Value.SetMapIndex(vkk, vvv)
					}
					// println("=============", kk, ToStr(vv))
				}
			}
		}

	}
	return
}

// ToStrArr ... []string
func ToStrArr(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return []string{}
	}
	sta := ToStrOrArr(val)
	if str, ok := sta.(string); ok {
		return []string{str}
	}
	return sta.([]string)
}

// ToStrOrArr ... string or []string
func ToStrOrArr(val string /*, bjs bool*/) any {
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}
	if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
		return val[1 : len(val)-1]
	}
	if !strings.HasPrefix(val, "[") || !strings.HasSuffix(val, "]") {
		return val
	}
	// if bjs {
	// 	arr := []any{}
	// 	if err := json.Unmarshal([]byte(val), &arr); err != nil {
	// 		return []string{val} // 无法解析
	// 	}
	// 	ass := make([]string, len(arr))
	// 	for vi, vv := range arr {
	// 		ass[vi] = fmt.Sprintf("%v", vv)
	// 	}
	// 	return ass
	// }

	val = val[1 : len(val)-1]
	arr := []string{}
	buf := strings.Builder{}
	stt := 0
	spd := false // 转义状态（\开头）
	for _, vc := range val {
		switch stt {
		case 0: // 等待元素开始
			if vc == ' ' || vc == ',' {
				continue // 跳过空格和逗号，等待元素开始
			}
			// 进入对应状态
			switch vc {
			case '"':
				stt = 1
			case '\'':
				stt = 2
			default:
				stt = 3
				buf.WriteRune(vc) // 非引号元素直接写入
			}
		case 1: // 双引号
			if spd {
				// 处理转义字符：\" 或 \\
				if vc == '"' || vc == '\\' {
					buf.WriteRune(vc)
				} else {
					// 非转义字符，保留\和原字符（如\n）
					buf.WriteRune('\\')
					buf.WriteRune(vc)
				}
				spd = false
				continue
			}
			// 未转义状态
			switch vc {
			case '\\':
				spd = true // 下一个字符需要转义
			case '"':
				// 双引号闭合，结束当前元素
				arr = append(arr, strings.TrimSpace(buf.String()))
				buf.Reset()
				stt = 0
			default:
				buf.WriteRune(vc)
			}
		case 2: // 单引号
			if spd {
				// 处理转义字符：\' 或 \\
				if vc == '\'' || vc == '\\' {
					buf.WriteRune(vc)
				} else {
					// 非转义字符，保留\和原字符（如\n）
					buf.WriteRune('\\')
					buf.WriteRune(vc)
				}
				spd = false
				continue
			}
			switch vc {
			case '\\':
				spd = true // 下一个字符需要转义
			case '\'':
				// 单引号闭合，结束当前元素
				arr = append(arr, strings.TrimSpace(buf.String()))
				buf.Reset()
				stt = 0
			default:
				buf.WriteRune(vc)
			}
		case 3: // 非字符串
			// 非字符串元素（数字、布尔等），遇到逗号或结束时停止
			if vc == ',' || vc == ' ' {
				arr = append(arr, strings.TrimSpace(buf.String()))
				buf.Reset()
				stt = 0
			} else {
				buf.WriteRune(vc)
			}
		}
	}
	if stt != 0 && buf.Len() > 0 {
		arr = append(arr, strings.TrimSpace(buf.String()))
	}
	return arr
}

// StrToBV
func StrToBV(typ reflect.Type, val string) (any, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, errors.New("<nil>")
	}
	return ToBasicValue(typ, ToStrArr(val))
}

// ToBasicValue ...
func ToBasicValue(typ reflect.Type, val []string) (any, error) {
	if len(val) == 0 {
		return nil, errors.New("<nil>")
	}
	str := val[0]
	switch typ.Kind() {
	case reflect.String:
		return str, nil
	case reflect.Bool:
		return strconv.ParseBool(str)
	case reflect.Int:
		return strconv.Atoi(str)
	case reflect.Int32:
		vvv, err := strconv.ParseInt(str, 10, 32)
		return int32(vvv), err
	case reflect.Int64:
		vvv, err := strconv.ParseInt(str, 10, 64)
		return vvv, err
	case reflect.Uint:
		vvv, err := strconv.ParseUint(str, 10, 64)
		return uint(vvv), err
	case reflect.Uint32:
		vvv, err := strconv.ParseUint(str, 10, 32)
		return uint32(vvv), err
	case reflect.Uint64:
		vvv, err := strconv.ParseUint(str, 10, 64)
		return vvv, err
	case reflect.Float32:
		vvv, err := strconv.ParseFloat(str, 32)
		return float32(vvv), err
	case reflect.Float64:
		vvv, err := strconv.ParseFloat(str, 64)
		return vvv, err
	case reflect.Slice:
		ccz := typ.Elem()
		switch ccz.Kind() {
		case reflect.String:
			return val, nil
		case reflect.Uint8:
			return []byte(str), nil
		}
		vvv := reflect.MakeSlice(typ, 0, 0)
		for _, vv := range val {
			vva, err := ToBasicValue(ccz, []string{vv})
			if err != nil {
				return nil, err
			}
			vvv = reflect.Append(vvv, reflect.ValueOf(vva))
		}
		return vvv.Interface(), nil
	case reflect.Array:
		ccz := typ.Elem()
		switch ccz.Kind() {
		case reflect.String:
			return val, nil
		case reflect.Uint8:
			return []byte(str), nil
		}
		vvv := reflect.New(typ).Elem() // 创建数组
		for vi, vv := range val {
			vva, err := ToBasicValue(ccz, []string{vv})
			if err != nil {
				return nil, err
			}
			vvv.Index(vi).Set(reflect.ValueOf(vva))
		}
		return vvv.Interface(), nil
	}
	return nil, errors.New("<" + typ.String() + "> type not supported")
}
