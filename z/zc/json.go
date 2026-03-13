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
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"
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

// ---------------------------------------------------------------------------------------

func TrimYamlString(str string) string {
	sbr := strings.Builder{}
	for line := range strings.SplitSeq(str, "\n") {
		sbr.WriteString(strings.TrimRightFunc(line, unicode.IsSpace))
		sbr.WriteRune('\n')
	}
	return strings.TrimRightFunc(sbr.String(), unicode.IsSpace)
}

// ---------------------------------------------------------------------------------------

// 只有类型匹配才返回，否则直接 def
func MapDef[T any](src map[string]any, key string, def T) T {
	val := MapItr(src, key, false, nil)
	if val == nil {
		return def
	}
	if vv, ok := val.(T); ok {
		return vv
	}
	return def
}

// 将任意类型转换为 T 类型, 尽量转换， 这里处理了 string 和 number bool 类型间的附加关系
func MapAny[T any](src map[string]any, key string, def T) T {
	val := MapItr(src, key, false, nil)
	return ToAny(val, def)
}

// 将任意类型转换为 int 类型
func MapInt(src map[string]any, key string, def int) int {
	val := MapItr(src, key, false, nil)
	return ToInt(val, def)
}

// 从 map 中获取字段的值， 原始数据， 同 MapItr(src, key, false, nil) 操作
func MapVal(src map[string]any, key string) any {
	return MapItr(src, key, false, nil)
}

// 覆盖 map 中的值，如果 val 为 nil 则删除字段
// 多用于 删除 或 已有字段覆盖, 可用户新增，但是父路径不存在，无法新增
func MapSet(src map[string]any, key string, val any) any {
	return MapItr(src, key, false, func() any { return val })
}

// 覆盖 map 中的值，如果路径不存在，创建字段，前提 val 不为 nil，
// 如果要创建， 数组必须是 -0(追加)， 否则不会创建字段， 多用于新增， 可修复父路径
// -0 表示创建 []any, 否则创建 map[ string ]any，如果使用数组，存在路径失败的风险
func MapNew(src map[string]any, key string, val any) any {
	return MapItr(src, key, val != nil, func() any { return val })
}

// 支持 key=containers.-1.env.[.name=EXT_CFG_HOST].value， Iterator or Traverse
func MapItr(src any, key string, nok bool, vfn func() any) any {
	paths := MapParserPaths(key) // strings.Split(key, ".")
	// z.Println("[_mapkey_]: map traverse: keys=", keys)
	last := len(paths) - 1
	curr := src
	var mvfn func(any) = nil
	for indx, ikey := range paths {
		if curr == nil {
			return nil
		}
		if m, ok := curr.(map[any]any); ok {
			mk := FindByFieldInMap(m, ikey, any(ikey))
			curr, _ = m[mk]
			if indx == last && vfn != nil {
				if v := vfn(); v != nil {
					m[mk] = v
				} else {
					delete(m, mk)
				}
			} else if curr == nil && nok && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := paths[indx+1]; next == "-0" {
					curr = []any{}
					m[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					m[mk] = curr // 创建字段
				}
			}
			mvfn = func(v any) { m[mk] = v }
		} else if m, ok := curr.(map[string]any); ok {
			mk := FindByFieldInMap(m, ikey, ikey)
			curr, _ = m[mk] // 通过 key 获取内容
			if indx == last && vfn != nil {
				if v := vfn(); v != nil {
					m[mk] = v
				} else {
					delete(m, mk)
				}
			} else if curr == nil && nok && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := paths[indx+1]; next == "-0" {
					curr = []any{}
					m[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					m[mk] = curr // 创建字段
				}
			}
			mvfn = func(v any) { m[mk] = v }
		} else if a, ok := curr.([]any); ok {
			curr = nil
			var avfn func(int) = nil
			if ikey != "-0" && indx == last && vfn != nil {
				avfn = func(i int) {
					if v := vfn(); v != nil {
						a[i] = v   // 更新字段
						mvfn = nil // 无需调用
					} else if i == 0 {
						a = a[1:] // 删除第一个
					} else if i == len(a)-1 {
						a = a[:i] // 删除最后一个
					} else {
						a = append(a[:i], a[i+1:]...) // 中间删除
					}
					if mvfn != nil {
						mvfn(a)
					}
				}
			}
			if ikey == "-0" {
				// 末尾追加数据
				if indx == last && vfn != nil {
					if v := vfn(); v != nil {
						a = append(a, v)
						if mvfn != nil {
							mvfn(a)
						}
						i := len(a) - 1
						mvfn = func(v any) { a[i] = v }
					}
				} else if nok && indx < last {
					// 未到末尾，已经没有值了， 创建字段
					if next := paths[indx+1]; next == "-0" {
						curr = []any{}
						a = append(a, curr) // 创建数组
						if mvfn != nil {
							mvfn(a)
						}
						i := len(a) - 1
						mvfn = func(v any) { a[i] = v }
					} else {
						curr = map[string]any{}
						a = append(a, curr) // 创建字段
						if mvfn != nil {
							mvfn(a)
						}
						i := len(a) - 1
						mvfn = func(v any) { a[i] = v }
					}
				}
			} else if strings.HasPrefix(ikey, "-") {
				// 倒序检索数据
				ak := ikey[1:]
				if i, err := strconv.Atoi(ak); err != nil {
					// 数字转换失败
				} else if i > 0 && i <= len(a) { // 倒序检索
					j := len(a) - i
					curr = a[j]
					if avfn != nil {
						avfn(j)
					}
					mvfn = func(v any) { a[j] = v }
				}
			} else if strings.HasPrefix(ikey, ".") {
				// 通过属性检索数据
				if i := FindByFieldInArr(a, ikey); i >= 0 {
					curr = a[i]
					if avfn != nil {
						avfn(i)
					}
					mvfn = func(v any) { a[i] = v }
				}
			} else {
				if i, err := strconv.Atoi(ikey); err != nil {
					// 数字转换失败
				} else if i < len(a) {
					curr = a[i]
					if avfn != nil {
						avfn(i)
					}
					mvfn = func(v any) { a[i] = v }
				}
			}
		} else {
			// 其他类型暂不支持
			curr = nil
		}
	}
	return curr
}

// 支持 key=x.[a.b.c].z.[.name=xxx].v 格式
func MapParserPaths(path string) []string {
	// 循环方式实现
	paths := []string{}
	if path == "" {
		return paths
	}
	curr := path
	for len(curr) > 0 {
		// 处理开头连续的点
		if curr[0] == '.' {
			curr = curr[1:]
			continue
		}
		switch curr[0] {
		case '[':
			// 匹配方括号模式
			idx := strings.IndexByte(curr, ']')
			if idx < 0 {
				// 没有匹配的右括号，直接添加剩余部分
				paths = append(paths, curr)
				curr = ""
				continue
			}
			// 提取方括号内的内容
			paths = append(paths, curr[1:idx])
			// 跳过 ] 继续处理后续
			curr = curr[idx+1:]
		default:
			// 匹配点分隔模式
			idx := strings.IndexByte(curr, '.')
			if idx < 0 {
				// 没有后续点，添加剩余部分
				paths = append(paths, curr)
				curr = ""
				continue
			}
			// 提取点前面的内容
			paths = append(paths, curr[:idx])
			// 跳过点继续处理后续
			curr = curr[idx+1:]
		}
	}
	return paths
}

// 推荐使用 MapParserPaths 函数， 原则循环优先于递归
func MapParserPath2(path string) []string {
	// 递归方式实现
	if path == "" {
		return []string{}
	}
	if path[0] == '.' {
		path = path[1:] // 删除 .
	}
	if path == "" {
		return []string{}
	}
	if path[0] == '[' {
		if idx := strings.IndexByte(path, ']'); idx < 0 {
			return []string{path} // 后面不存在 [ ] 了
		} else {
			curr := path[1:idx]
			next := MapParserPath2(path[idx+1:])
			return append([]string{curr}, next...)
		}
	}
	if idx := strings.IndexByte(path, '.'); idx < 0 {
		return []string{path} // 后面不存在 . 了
	} else {
		curr := path[:idx]
		next := MapParserPath2(path[idx:])
		return append([]string{curr}, next...) // slices.Insert()
	}
}

// 从源 map 中查找字段， 更具字段属性进行匹配， key 必须是 .name=xxx | .name=^reg 格式
func FindByFieldInMap[K comparable](src map[K]any, key string, def K) K {
	if len(key) == 0 || key[0] != '.' || strings.IndexByte(key, '=') <= 0 {
		return def
	}
	// 使用属性匹配进行查询
	k2 := strings.SplitN(key[1:], "=", 2)
	var kre *regexp.Regexp // ^ 开头启动正则表达式匹配
	if sz := len(k2[1]); sz > 0 && k2[1][0] == '^' {
		kre, _ = regexp.Compile(k2[1]) // 正则表达式失败，也会使用 str 匹配
	}
	for ck, v := range src {
		var v3 any
		if v2, ok := v.(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := v.(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 == nil {
			continue // 没有属性
		} else if v3 == k2[1] {
			return ck // 匹配到结果, 替换原有的 key
		} else if kre == nil {
			continue // 没有正则
		} else if str, sok := v3.(string); sok && kre.MatchString(str) {
			return ck // 匹配到结果, 替换原有的 key
		}
	}
	return def
}

// 从数组中查找字段， 更具字段属性进行匹配， key 必须是 .name=xxx | .name=^reg 格式
func FindByFieldInArr(src []any, key string) int {
	if len(key) == 0 || key[0] != '.' || strings.IndexByte(key, '=') <= 0 {
		return -1
	}
	// 通过属性检索数据
	k2 := strings.SplitN(key[1:], "=", 2)
	var kre *regexp.Regexp // ^ 开头启动正则表达式匹配
	if sz := len(k2[1]); sz > 0 && k2[1][0] == '^' {
		kre, _ = regexp.Compile(k2[1]) // 正则表达式失败，也会使用 str 匹配
	}
	for i, v := range src {
		var v3 any = nil
		if v2, ok := v.(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := v.(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 == nil {
			continue
		} else if v3 == k2[1] {
			return i // 匹配到结果
		} else if kre == nil {
			continue
		} else if str, sok := v3.(string); sok && kre.MatchString(str) {
			return i // 匹配到结果
		}
	}
	return -1
}

// ---------------------------------------------------------------------------------------

// any 转换为指定类型
func ToAny[T any](val any, def T) T {
	if val == nil {
		return def
	}
	if vv, ok := val.(T); ok {
		return vv
	}
	// 类型处理
	vdata := reflect.ValueOf(val)
	if vdata.Kind() == reflect.Pointer {
		if vdata.IsNil() {
			return def
		}
		vdata = vdata.Elem()
	}
	vtype := reflect.TypeFor[T]()
	// 调用系统内部类型直接转换
	if vdata.Type().ConvertibleTo(vtype) {
		return vdata.Convert(vtype).Interface().(T)
	}
	// 使用 fmt 将任意类型转换为 string 类型
	if vtype.Kind() == reflect.String {
		return any(fmt.Sprintf("%v", vdata.Interface())).(T)
	}
	// 支持字符串类型的数字转换
	if vdata.Kind() == reflect.String {
		str := vdata.String()
		if str == "<nil>" || str == "<null>" {
			return def // 特殊的“空”标记
		}
		switch vtype.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if num, err := strconv.ParseInt(str, 10, 64); err == nil {
				return reflect.ValueOf(num).Convert(vtype).Interface().(T)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if num, err := strconv.ParseUint(str, 10, 64); err == nil {
				return reflect.ValueOf(num).Convert(vtype).Interface().(T)
			}
		case reflect.Float32, reflect.Float64:
			if num, err := strconv.ParseFloat(str, 64); err == nil {
				return reflect.ValueOf(num).Convert(vtype).Interface().(T)
			}
		case reflect.Bool:
			// case bool
			switch str {
			case "0", "no", "off", "N", "否", "禁用", "disable":
				return any(false).(T)
			case "1", "yes", "on", "Y", "是", "启用", "enable":
				return any(true).(T)
			}
		}
	}
	// bool 类型, 0: false, >0: true <0: def
	if vtype.Kind() == reflect.Bool {
		if vv := ToInt(val, -1); vv == 0 {
			return any(false).(T)
		} else if vv > 0 {
			return any(true).(T)
		}
	}
	return def
}

// any 转换为 int
func ToInt(val any, def int) int {
	if val == nil {
		return def
	}
	switch vv := val.(type) {
	case int:
		return vv
	case int8:
		return int(vv)
	case int16:
		return int(vv)
	case int32:
		return int(vv)
	case int64:
		return int(vv)
	case uint:
		return int(vv)
	case uint8:
		return int(vv)
	case uint16:
		return int(vv)
	case uint32:
		return int(vv)
	case uint64:
		return int(vv) // 注意：超大 uint64 转 int 可能溢出
	case float32:
		return int(vv) // 小数部分会被截断
	case float64:
		return int(vv) // 小数部分会被截断
	case string:
		// 可选：支持字符串形式的数字转换
		if num, err := strconv.Atoi(vv); err == nil {
			return num
		}
		return def
	default:
		return def
	}
}
