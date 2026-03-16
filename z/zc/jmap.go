package zc

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

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

// 支持 key=key.-1.env.[.name=(^)xxx].key.key， Iterator or Traverse or Recursion
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
		switch cur := curr.(type) {
		case map[any]any:
			mk := any(ikey)
			if mks := FindByFieldInMap(cur, ikey, true); len(mks) > 0 {
				mk = mks[0] // 优先检索
			}
			curr, _ = cur[mk]
			if indx == last && vfn != nil {
				if v, r := vfn(curr); r > 0 {
					cur[mk] = v
				} else if r < 0 {
					delete(cur, mk)
				}
			} else if curr == nil && fpv && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := keys[indx+1]; next == "-0" {
					curr = []any{}
					cur[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					cur[mk] = curr // 创建字段
				}
			}
			if vfn != nil {
				setv = func(v any) { cur[mk] = v }
			}
		case map[string]any:
			mk := ikey
			if mks := FindByFieldInMap(cur, ikey, true); len(mks) > 0 {
				mk = mks[0] // 优先检索
			}
			curr, _ = cur[mk] // 通过 key 获取内容
			if indx == last && vfn != nil {
				if v, r := vfn(curr); r > 0 {
					cur[mk] = v
				} else if r < 0 {
					delete(cur, mk)
				}
			} else if curr == nil && fpv && indx < last {
				// 未到末尾，已经没有值了， 创建字段
				if next := keys[indx+1]; next == "-0" {
					curr = []any{}
					cur[mk] = curr // 创建数组
				} else {
					curr = map[string]any{}
					cur[mk] = curr // 创建字段
				}
			}
			if vfn != nil {
				setv = func(v any) { cur[mk] = v }
			}
		case []any:
			curr = nil
			if ikey == "-0" {
				if vfn != nil {
					// 末尾追加数据
					if indx == last {
						if v, r := vfn(nil); r > 0 {
							cur = append(cur, v)
							if setv != nil {
								setv(cur)
							}
							i := len(cur) - 1
							setv = func(v any) { cur[i] = v }
						}
					} else if fpv && indx < last {
						// 未到末尾，已经没有值了， 创建字段
						if next := keys[indx+1]; next == "-0" {
							curr = []any{}
							cur = append(cur, curr) // 创建数组
						} else {
							curr = map[string]any{}
							cur = append(cur, curr) // 创建字段
						}
						if setv != nil {
							setv(cur)
						}
						ai := len(cur) - 1
						setv = func(v any) { cur[ai] = v }
					}
				}
			} else {
				ai := -1
				if strings.HasPrefix(ikey, "-") {
					// 倒序检索数据
					ak := ikey[1:]
					if i, err := strconv.Atoi(ak); err != nil {
						// 数字转换失败
					} else if i > 0 && i <= len(cur) { // 倒序检索
						ai = len(cur) - i
					}
				} else if strings.HasPrefix(ikey, ".") {
					// 通过属性检索数据
					ais := FindByFieldInArr(cur, ikey, true)
					if len(ais) > 0 {
						ai = ais[0]
					}
				} else {
					if i, err := strconv.Atoi(ikey); err != nil {
						// 数字转换失败
					} else if i >= 0 && i < len(cur) {
						ai = i
					}
				}
				if ai >= 0 {
					curr = cur[ai]
					if indx == last && vfn != nil {
						if v, r := vfn(cur[ai]); r > 0 {
							cur[ai] = v // 更新字段
						} else if r < 0 {
							if ai == 0 {
								cur = cur[1:] // 删除第一个
							} else if ai == len(cur)-1 {
								cur = cur[:ai] // 删除最后一个
							} else {
								cur = append(cur[:ai], cur[ai+1:]...) // 中间删除
							}
							if setv != nil {
								setv(cur)
							}
						}
					}
					if vfn != nil {
						setv = func(v any) { cur[ai] = v }
					}
				}
			}
		default:
			// 其他类型暂不支持
			curr = nil
		}
	}
	return curr
}

// ---------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------

// 检索属于规范的 key 列表和对应的值，返回 map[string]any, Iterator or Traverse or Recursion，
// MapKeyVar 和 MapKeyVal 功能相同。基于测试， MapKeyVal 效率会更好一些， 百万次查询，相差30%左右。
func MapKeyVal(src any, key string) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	keys := MapParserPaths(key)
	return MapKeyItr(src, keys...)
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any
func MapKeyItr(src any, keys ...string) map[string]any {
	result := make(map[string]any)
	// 边界条件处理，与原逻辑完全一致
	if len(keys) == 0 {
		return result
	}
	// 遍历栈元素：保存单次处理的上下文
	type node struct {
		from *node
		elem any      // 当前处理的对象
		path string   // 当前已拼接的路径
		keys []string // 剩余待匹配的key列表
		onex *bool    // 是否匹配到
	}
	// 数组查询结果
	type item struct {
		key string
		val any
	}
	// 初始化栈，放入初始参数
	path := ""
	stack := []*node{{elem: src, path: path, keys: keys}}
	hasx := false
	for len(stack) > 0 {
		// 弹出栈顶元素（LIFO，保证遍历顺序与原递归一致）
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if curr.onex != nil && *curr.onex {
			continue
		}
		// 没有剩余key，直接存入结果
		if len(curr.keys) == 0 {
			if curr.path != "" {
				result[curr.path] = curr.elem
				if hasx {
					// 通知上层 one '?' 模块，内容已经找到
					for curr.from != nil {
						curr = curr.from
						if curr.onex != nil {
							*curr.onex = true
						}
					}
				}
			}
			continue
		}
		ikey := curr.keys[0]
		var onex *bool
		if ikey == "?" {
			onez := false
			onex = &onez
			hasx = true
		}
		remainingKeys := curr.keys[1:]
		switch cur := curr.elem.(type) {
		case map[any]any:
			// 查找匹配的key
			mks := FindByFieldInMap(cur, ikey, false)
			if len(mks) == 0 {
				mks = []any{ikey}
			}
			// 倒序遍历保证执行顺序与原递归一致
			for i := len(mks) - 1; i >= 0; i-- {
				ma := mks[i]
				curVal, ok := cur[ma]
				if !ok {
					continue
				}
				// 拼接路径
				mk := fmt.Sprintf("%v", ma)
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if curr.path != "" {
					key = curr.path + "." + key
				}
				// 普通场景压入栈继续处理
				stack = append(stack, &node{
					from: curr,
					elem: curVal,
					path: key,
					keys: remainingKeys,
					onex: onex,
				})
			}
		case map[string]any:
			// 查找匹配的key
			mks := FindByFieldInMap(cur, ikey, false)
			if len(mks) == 0 {
				mks = []string{ikey}
			}
			// 倒序遍历保证执行顺序与原递归一致
			for i := len(mks) - 1; i >= 0; i-- {
				mk := mks[i]
				curVal, ok := cur[mk]
				if !ok {
					continue
				}
				// 拼接路径
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if curr.path != "" {
					key = curr.path + "." + key
				}
				// 普通场景压入栈继续处理
				stack = append(stack, &node{
					from: curr,
					elem: curVal,
					path: key,
					keys: remainingKeys,
					onex: onex,
				})
			}
		case []any:
			matched := []item{}
			if ikey == "-0" {
				continue // 忽略-0
			}
			switch {
			case strings.HasPrefix(ikey, "-"):
				// 负索引倒序检索
				ak := ikey[1:]
				i, err := strconv.Atoi(ak)
				if err == nil && i > 0 && i <= len(cur) {
					ai := len(cur) - i
					curVal := cur[ai]
					// 拼接路径
					key := strconv.Itoa(ai)
					if curr.path != "" {
						key = curr.path + "." + key
					}
					matched = append(matched, item{key, curVal})
				}
			case strings.HasPrefix(ikey, ".") || ikey == "*" || ikey == "?":
				// 按属性检索数组元素
				ais := FindByFieldInArr(cur, ikey, false)
				for _, ai := range ais {
					curVal := cur[ai]
					key := strconv.Itoa(ai)
					if curr.path != "" {
						key = curr.path + "." + key
					}
					matched = append(matched, item{key, curVal})
				}
			default:
				// 正索引检索
				i, err := strconv.Atoi(ikey)
				if err == nil && i >= 0 && i < len(cur) {
					curVal := cur[i]
					key := strconv.Itoa(i)
					if curr.path != "" {
						key = curr.path + "." + key
					}
					matched = append(matched, item{key, curVal})
				}
			}
			// 倒序遍历保证执行顺序与原递归一致
			for i := len(matched) - 1; i >= 0; i-- {
				item := matched[i]
				curVal := item.val
				key := item.key
				// 普通场景压入栈继续处理
				stack = append(stack, &node{
					from: curr,
					elem: curVal,
					path: key,
					keys: remainingKeys,
					onex: onex,
				})
			}
			// default: println(reflect.TypeOf(cur))
		}
	}
	return result
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any, Iterator or Traverse or Recursion，
// MapKeyVar 和 MapKeyVal 功能相同。基于测试，MapKeyVal 效率会更好一些， 百万次查询，相差30%左右。
func MapKeyVar(src any, key string) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	keys := MapParserPaths(key)
	return MapKeyRec_(src, "", keys...)
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any
func MapKeyRec(src any, keys ...string) map[string]any {
	return MapKeyRec_(src, "", keys...)
}

// 检索属于规范的 key 列表和对应的值，返回 map[string]any, Iterator or Traverse or Recursion
func MapKeyRec_(curr any, path string, keys ...string) map[string]any {
	if len(keys) == 0 && path == "" {
		return map[string]any{}
	}
	if len(keys) == 0 {
		return map[string]any{path: curr}
	}
	dest := map[string]any{} // 返回值列表
	ikey := keys[0]
	keys = keys[1:]
	one := ikey == "?"
	switch curr := curr.(type) {
	case map[any]any:
		mks := FindByFieldInMap(curr, ikey, false)
		if len(mks) == 0 {
			mks = []any{ikey} // 使用默认值到 key
		}
		for _, ma := range mks {
			mk := fmt.Sprintf("%v", ma)
			if cur, cok := curr[ma]; cok {
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyRec_(cur, key, keys...)
				if one && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		}
	case map[string]any:
		mks := FindByFieldInMap(curr, ikey, false)
		if len(mks) == 0 {
			mks = []string{ikey} // 使用默认值到 key
		}
		for _, mk := range mks {
			if cur, cok := curr[mk]; cok {
				key := mk
				if strings.IndexByte(key, '.') >= 0 {
					key = "[" + key + "]"
				}
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyRec_(cur, key, keys...)
				if one && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		}
	case []any:
		switch {
		case ikey == "-0":
			// ignore
		case strings.HasPrefix(ikey, "-"):
			// 倒序检索数据
			ak := ikey[1:]
			if i, err := strconv.Atoi(ak); err != nil {
				// 数字转换失败
			} else if i > 0 && i <= len(curr) { // 倒序检索
				ai := len(curr) - i
				cur := curr[ai]
				key := strconv.Itoa(ai)
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyRec_(cur, key, keys...)
				if one && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		case strings.HasPrefix(ikey, ".") || ikey == "*" || ikey == "?":
			// 通过属性检索数据
			ais := FindByFieldInArr(curr, ikey, false)
			for _, ai := range ais {
				cur := curr[ai]
				key := strconv.Itoa(ai)
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyRec_(cur, key, keys...)
				if one && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		default:
			if i, err := strconv.Atoi(ikey); err != nil {
				// 数字转换失败
			} else if i >= 0 && i < len(curr) {
				ai := i
				cur := curr[ai]
				key := strconv.Itoa(ai)
				if path != "" {
					key = path + "." + key
				}
				dst := MapKeyRec_(cur, key, keys...)
				if one && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				for k, v := range dst {
					dest[k] = v
				}
			}
		}
	}
	return dest
}

// ---------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------

// 支持 key=x.[a.b.c].z.[.name=xxx].x[.name=zzz].v 格式
func MapParserPaths(path string) []string {
	paths := []string{}
	if path == "" {
		return paths
	}
	curr := []rune(path)
	n := len(curr)
	i := 0
	for i < n {
		// 跳过开头连续的点
		for i < n && curr[i] == '.' {
			i++
		}
		if i >= n {
			break
		}
		// 处理普通字符开头的段，支持后面跟方括号筛选条件
		j := i
		d := 0
		for j < n {
			if curr[j] == '[' {
				d++
			} else if curr[j] == ']' {
				d--
			} else if curr[j] == '.' && d == 0 {
				// 不在方括号内的点才是分隔符
				break
			}
			j++
		}
		// 提取完整路径段（包含后面的所有方括号筛选条件）
		a := string(curr[i:j])
		if s := len(a); s > 1 && a[0] == '[' && a[s-1] == ']' {
			a = a[1 : s-1]
		}
		paths = append(paths, a)
		i = j
	}
	// println(ToStr(paths))
	return paths
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
	if len(key) == 0 || strings.IndexByte(key, '=') <= 0 {
		return ks
	}
	if idx := strings.Index(key, "[."); idx > 0 && key[len(key)-1] == ']' {
		// key[.name=xxx] 已知 key， 确定 key 对应的内容
		ck, ok := any(key[:idx]).(K) // key 类型转换
		if !ok {
			return ks
		}
		k2 := strings.SplitN(key[idx+2:len(key)-1], "=", 2)
		if len(k2) != 2 {
			return ks
		}
		var kre *regexp.Regexp // ^ 开头启动正则表达式匹配
		if sz := len(k2[1]); sz > 0 && k2[1][0] == '^' {
			kre, _ = regexp.Compile(k2[1]) // 正则表达式失败，也会使用 str 匹配
		}
		v := src[ck] // 直接指定
		var v3 any
		if k2[0] == "" {
			v3 = v
		} else if v2, ok := v.(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := v.(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 != nil && IsMatchByField(v3, k2[1], kre) {
			ks = append(ks, ck) // 匹配到结果
		}
		return ks
	}
	// [.name=xxx] 格式， 需要寻找 key 对应的内容
	if key[0] != '.' {
		return ks
	}
	// 使用属性匹配进行查询
	k2 := strings.SplitN(key[1:], "=", 2)
	if len(k2) != 2 {
		return ks
	}
	var kre *regexp.Regexp // ^ 开头启动正则表达式匹配
	if sz := len(k2[1]); sz > 0 && k2[1][0] == '^' {
		kre, _ = regexp.Compile(k2[1]) // 正则表达式失败，也会使用 str 匹配
	}
	for ck, v := range src {
		var v3 any
		if k2[0] == "" {
			v3 = v
		} else if v2, ok := v.(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := v.(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 != nil && IsMatchByField(v3, k2[1], kre) {
			ks = append(ks, ck) // 匹配到结果
			if one {
				break
			}
		}
	}
	return ks
}

// 执行内容匹配， 暂时只支持 int, int64, float64, string, bool 类型
func IsMatchByField(val any, src string, kre *regexp.Regexp) bool {
	if src == val {
		return true
	}
	switch val := val.(type) {
	case string:
		if siz := len(src); siz > 1 && src[0] == '\'' && src[siz-1] == '\'' && val == src[1:siz-1] {
			return true // 特殊写法，只匹配字符串情况
		}
		if kre != nil && kre.MatchString(val) {
			return true
		}
		// if len(src) > 0 && src[0] == '~' {
		// 	return strings.Contains(val, src[1:])
		// }
	case bool:
		if src == "true" || src == "false" {
			return val == (src == "true")
		}
	case int:
		key := strings.TrimPrefix(src, "int.") // 解决类型匹配问题
		if len(key) > 0 {
			if key[0] == '>' {
				if num, err := strconv.Atoi(key[1:]); err == nil && val > num {
					return true
				}
			} else if key[0] == '<' {
				if num, err := strconv.Atoi(key[1:]); err == nil && val < num {
					return true
				}
			} else if num, err := strconv.Atoi(key); err == nil && val == num {
				return true
			}
		}
	// 暂时 忽略 int8 和 int16, 减少计算量， 简化判断逻辑
	case int32:
		key := strings.TrimPrefix(src, "i32.") // 解决类型匹配问题
		if len(key) > 0 {
			if key[0] == '>' {
				if num, err := strconv.ParseInt(key[1:], 10, 64); err == nil && int64(val) > num {
					return true
				}
			} else if key[0] == '<' {
				if num, err := strconv.ParseInt(key[1:], 10, 64); err == nil && int64(val) < num {
					return true
				}
			} else if num, err := strconv.ParseInt(key, 10, 64); err == nil && int64(val) == num {
				return true
			}
		}
	case int64:
		key := strings.TrimPrefix(src, "i64.") // 解决类型匹配问题
		if len(key) > 0 {
			if key[0] == '>' {
				if num, err := strconv.ParseInt(key[1:], 10, 64); err == nil && val > num {
					return true
				}
			} else if key[0] == '<' {
				if num, err := strconv.ParseInt(key[1:], 10, 64); err == nil && val < num {
					return true
				}
			} else if num, err := strconv.ParseInt(key, 10, 64); err == nil && val == num {
				return true
			}
		}
	case float32:
		key := strings.TrimPrefix(src, "f32.") // 解决类型匹配问题
		if len(key) > 0 {
			if key[0] == '>' {
				if num, err := strconv.ParseFloat(key[1:], 64); err == nil && float64(val) > num {
					return true
				}
			} else if key[0] == '<' {
				if num, err := strconv.ParseFloat(key[1:], 64); err == nil && float64(val) < num {
					return true
				}
			} else if num, err := strconv.ParseFloat(key, 64); err == nil && float64(val) == num {
				return true
			}
		}
	case float64:
		// 在反序列化，存在 int, int64 -> float64 情况, 暂时不考虑这种情况的出现
		// if strings.HasPrefix(src, "int.") || strings.HasPrefix(src, "i64.") {
		// 	key = src[4:] // 修正这类问题， 强制使用 float64， 匹配， 暂时为决定引入此条规则
		// }
		key := strings.TrimPrefix(src, "f64.") // 解决类型匹配问题
		if len(key) > 0 {
			if key[0] == '>' {
				if num, err := strconv.ParseFloat(key[1:], 64); err == nil && val > num {
					return true
				}
			} else if key[0] == '<' {
				if num, err := strconv.ParseFloat(key[1:], 64); err == nil && val < num {
					return true
				}
			} else if num, err := strconv.ParseFloat(key, 64); err == nil && val == num {
				return true
			}
		}
	}
	return false
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
	if len(k2) != 2 {
		return ks
	}
	var kre *regexp.Regexp // ^ 开头启动正则表达式匹配
	if sz := len(k2[1]); sz > 0 && k2[1][0] == '^' {
		kre, _ = regexp.Compile(k2[1]) // 正则表达式失败，也会使用 str 匹配
	}
	for i, v := range src {
		var v3 any = nil
		if k2[0] == "" {
			v3 = v
		} else if v2, ok := v.(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := v.(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 != nil && IsMatchByField(v3, k2[1], kre) {
			// 匹配到结果
			ks = append(ks, i)
			if one {
				break
			}
		}
	}
	return ks
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
