package zc

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// 检索属于规范的 key 列表和对应的值，返回 map[string]any
func MapKey(src any, key string) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	keys := MapParserPaths(key)
	return MapKeyTrs(src, "", keys...)
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any
func MapKeys(src any, keys ...string) map[string]any {
	return MapKeyTrs(src, "", keys...)
}

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
func MapGet(src map[string]any, key string) any {
	return MapItr(src, key, false, nil)
}

// 从 map 中获取字段的值， 获取所有匹配的数据
func MapAll(src map[string]any, key string) []any {
	return nil
}

// 覆盖 map 中的值，如果 val 为 nil 则删除字段，
// 多用于 删除 或 已有字段覆盖, 可用户新增，但是父路径不存在，无法新增。
func MapSet(src map[string]any, key string, val any) any {
	return MapItr(src, key, false, func(_ any) (any, int8) { return val, If[int8](val == nil, -1, 1) })
}

// 覆盖 map 中的值，如果路径不存在，创建字段，前提 val 不为 nil，
// 如果要创建， 数组必须是 -0(追加)， 否则不会创建字段， 多用于新增， 可修复父路径，
// -0 表示创建 []any, 否则创建 map[ string ]any，如果使用数组，存在路径失败的风险。
func MapNew(src map[string]any, key string, val any) any {
	return MapItr(src, key, val != nil, func(_ any) (any, int8) { return val, If[int8](val == nil, -1, 1) })
}

// ---------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------

// 支持 key=key.-1.env.[.name=(^)xxx].key.key， Iterator or Traverse
// cover: = 0 忽略， > 1 覆盖， < 0 删除,
// fnk: fix path value, 修复路径上的所有值。
func MapItr(src any, key string, fpv bool, vfn func(any) (value any, cover int8)) any {
	if src == nil {
		return nil
	}
	keys := MapParserPaths(key) // strings.Split(key, ".")
	if len(keys) == 0 {
		return src
	}
	last := len(keys) - 1    // 末尾标记
	var setv func(any) = nil // 赋值回调
	curr := src
	for indx, ikey := range keys {
		if curr == nil {
			return nil
		}
		if mm, ok := curr.(map[any]any); ok {
			mk := any(ikey)
			if mks := FindByFieldInMap(mm, ikey, true); len(mks) > 0 {
				mk = mks[0] // 优先检索
			}
			curr, _ = mm[mk]
			if indx == last && vfn != nil {
				if v, r := vfn(curr); r > 0 {
					mm[mk] = v
				} else if r < 0 {
					delete(mm, mk)
				}
			} else if curr == nil && fpv && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := keys[indx+1]; next == "-0" {
					curr = []any{}
					mm[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					mm[mk] = curr // 创建字段
				}
			}
			if vfn != nil {
				setv = func(v any) { mm[mk] = v }
			}
		} else if mm, ok := curr.(map[string]any); ok {
			mk := ikey
			if mks := FindByFieldInMap(mm, ikey, true); len(mks) > 0 {
				mk = mks[0] // 优先检索
			}
			curr, _ = mm[mk] // 通过 key 获取内容
			if indx == last && vfn != nil {
				if v, r := vfn(curr); r > 0 {
					mm[mk] = v
				} else if r < 0 {
					delete(mm, mk)
				}
			} else if curr == nil && fpv && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := keys[indx+1]; next == "-0" {
					curr = []any{}
					mm[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					mm[mk] = curr // 创建字段
				}
			}
			if vfn != nil {
				setv = func(v any) { mm[mk] = v }
			}
		} else if aa, ok := curr.([]any); ok {
			curr = nil
			if ikey == "-0" {
				if vfn != nil {
					// 末尾追加数据
					if indx == last {
						if v, r := vfn(nil); r > 0 {
							aa = append(aa, v)
							if setv != nil {
								setv(aa)
							}
							i := len(aa) - 1
							setv = func(v any) { aa[i] = v }
						}
					} else if fpv && indx < last {
						// 未到末尾，已经没有值了， 创建字段
						if next := keys[indx+1]; next == "-0" {
							curr = []any{}
							aa = append(aa, curr) // 创建数组
						} else {
							curr = map[string]any{}
							aa = append(aa, curr) // 创建字段
						}
						if setv != nil {
							setv(aa)
						}
						ai := len(aa) - 1
						setv = func(v any) { aa[ai] = v }
					}
				}
			} else {
				ai := -1
				if strings.HasPrefix(ikey, "-") {
					// 倒序检索数据
					ak := ikey[1:]
					if i, err := strconv.Atoi(ak); err != nil {
						// 数字转换失败
					} else if i > 0 && i <= len(aa) { // 倒序检索
						ai = len(aa) - i
					}
				} else if strings.HasPrefix(ikey, ".") {
					// 通过属性检索数据
					ais := FindByFieldInArr(aa, ikey, true)
					if len(ais) > 0 {
						ai = ais[0]
					}
				} else {
					if i, err := strconv.Atoi(ikey); err != nil {
						// 数字转换失败
					} else if i >= 0 && i < len(aa) {
						ai = i
					}
				}
				if ai >= 0 {
					curr = aa[ai]
					if indx == last && vfn != nil {
						if v, r := vfn(aa[ai]); r > 0 {
							aa[ai] = v // 更新字段
						} else if r < 0 {
							if ai == 0 {
								aa = aa[1:] // 删除第一个
							} else if ai == len(aa)-1 {
								aa = aa[:ai] // 删除最后一个
							} else {
								aa = append(aa[:ai], aa[ai+1:]...) // 中间删除
							}
							if setv != nil {
								setv(aa)
							}
						}
					}
					if vfn != nil {
						setv = func(v any) { aa[ai] = v }
					}
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
// src: 检索的 map, key: 属性字段， one: 是否只返回一个结果, 比 key = *, ? 优先级高
func FindByFieldInMap[K comparable](src map[K]any, key string, one bool) []K {
	ks := []K{}
	if key == "*" || key == "?" {
		// 匹配所有字段, * 所有， ？匹配到1个就返回
		for k := range src {
			ks = append(ks, k)
			if one {
				break
			}
		}
		return ks
	}
	if len(key) == 0 || key[0] != '.' || strings.IndexByte(key, '=') <= 0 {
		return ks
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
			// 匹配到结果
			ks = append(ks, ck)
			if one {
				break
			}
		} else if kre == nil {
			continue // 没有正则
		} else if str, sok := v3.(string); sok && kre.MatchString(str) {
			// 匹配到结果
			ks = append(ks, ck)
			if one {
				break
			}
		}
	}
	return ks
}

// 从数组中查找字段， 更具字段属性进行匹配， key 必须是 .name=xxx | .name=^reg 格式
// src: 检索的 数组, key: 属性字段， one: 是否只返回一个结果, 比 key = *, ? 优先级高
func FindByFieldInArr(src []any, key string, one bool) []int {
	ks := []int{}
	if key == "*" || key == "?" {
		// 匹配所有
		for i := range src {
			ks = append(ks, i)
			if one {
				break
			}
		}
		return ks
	}
	if len(key) == 0 || key[0] != '.' || strings.IndexByte(key, '=') <= 0 {
		return ks
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
			// 匹配到结果
			ks = append(ks, i)
			if one {
				break
			}
		} else if kre == nil {
			continue
		} else if str, sok := v3.(string); sok && kre.MatchString(str) {
			// 匹配到结果
			ks = append(ks, i)
			if one {
				break
			}
		}
	}
	return ks
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any, Iterator or Traverse
func MapKeyTrs(curr any, path string, keys ...string) map[string]any {
	if len(keys) == 0 && path == "" {
		return map[string]any{}
	}
	if len(keys) == 0 {
		return map[string]any{path: curr}
	}
	dest := map[string]any{} // 返回值列表
	ikey := keys[0]
	keys = keys[1:]
	ione := ikey == "?"
	if mm, ok := curr.(map[any]any); ok {
		mks := FindByFieldInMap(mm, ikey, false)
		if len(mks) == 0 {
			mks = []any{ikey} // 使用默认值到 key
		}
		for _, ma := range mks {
			mk := fmt.Sprintf("%v", ma)
			if cur, cok := mm[ma]; cok {
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyTrs(cur, key, keys...)
				if ione && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		}
	} else if mm, ok := curr.(map[string]any); ok {
		mks := FindByFieldInMap(mm, ikey, false)
		if len(mks) == 0 {
			mks = []string{ikey} // 使用默认值到 key
		}
		for _, mk := range mks {
			if cur, cok := mm[mk]; cok {
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyTrs(cur, key, keys...)
				if ione && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		}
	} else if aa, ok := curr.([]any); ok {
		curr = nil
		if ikey == "-0" {
			// ignore
		} else {
			if strings.HasPrefix(ikey, "-") {
				// 倒序检索数据
				ak := ikey[1:]
				if i, err := strconv.Atoi(ak); err != nil {
					// 数字转换失败
				} else if i > 0 && i <= len(aa) { // 倒序检索
					ai := len(aa) - i
					cur := aa[ai]
					key := strconv.Itoa(ai)
					if path != "" {
						key = path + "." + key
					}
					dst := MapKeyTrs(cur, key, keys...)
					if ione && len(dst) > 0 {
						return dst // 找到一个就返回
					}
					for k, v := range dst {
						dest[k] = v
					}
				}
			} else if strings.HasPrefix(ikey, ".") {
				// 通过属性检索数据
				ais := FindByFieldInArr(aa, ikey, true)
				for _, ai := range ais {
					cur := aa[ai]
					key := strconv.Itoa(ai)
					if path != "" {
						key = path + "." + key
					}
					dst := MapKeyTrs(cur, key, keys...)
					if ione && len(dst) > 0 {
						return dst // 找到一个就返回
					}
					for k, v := range dst {
						dest[k] = v
					}
				}
			} else {
				if i, err := strconv.Atoi(ikey); err != nil {
					// 数字转换失败
				} else if i >= 0 && i < len(aa) {
					ai := i
					cur := aa[ai]
					key := strconv.Itoa(ai)
					if path != "" {
						key = path + "." + key
					}
					dst := MapKeyTrs(cur, key, keys...)
					if ione && len(dst) > 0 {
						return dst // 找到一个就返回
					}
					for k, v := range dst {
						dest[k] = v
					}
				}
			}
		}
	}
	return dest
}

// ---------------------------------------------------------------------------------------
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
