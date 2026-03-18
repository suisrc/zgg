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
	val := MapIterator(src, false, nil, key)
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
	return ToAny(MapIterator(src, false, nil, key), def)
}

// 将任意类型转换为 int 类型
func MapInt(src map[string]any, key string, def int) int {
	return ToInt(MapIterator(src, false, nil, key), def)
}

// 从 map 中获取字段的值， 原始数据， 同 MapItr(src, key, false, nil) 操作
func MapGet(src map[string]any, key string) any {
	return MapIterator(src, false, nil, key)
}

// 覆盖 map 中的值，如果 val 为 nil 则删除字段，
// 多用于 删除 或 已有字段覆盖, 可用户新增，但是父路径不存在，无法新增。
func MapSet(src map[string]any, key string, val any) any {
	return MapIterator(src, false, func(_ string, _ any) (any, int8) { return val, If[int8](val == nil, -1, 1) }, key)
}

// 覆盖 map 中的值，如果路径不存在，创建字段，前提 val 不为 nil，
// 如果要创建， 数组必须是 -0(追加)， 否则不会创建字段， 多用于新增， 可修复父路径，
// -0 表示创建 []any, 否则创建 map[ string ]any，如果使用数组，存在路径失败的风险。
func MapNew(src map[string]any, key string, val any) any {
	return MapIterator(src, true, func(_ string, _ any) (any, int8) { return val, 1 }, key)
}

// 删除 map 中的值， 如果是基础类型，可以用 MapSet(src, key, nil) 进行删除， 该方法专用于集合对象删除
func MapEmp[T ~string | ~[]any | ~map[any]any | ~map[string]any | ~chan any](src map[string]any, key string) any {
	return MapIterator(src, false, func(_ string, val any) (any, int8) { return nil, If[int8](val == nil || len(val.(T)) == 0, -1, 0) }, key)
}

// ---------------------------------------------------------------------------------------

// [读写模式], 使用循环的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name， Iterator or Traverse or Recursion
func MapIterator(src any, fpv bool, vfn func(string, any) (value any, cover int8), keys ...string) any {
	if len(keys) == 0 || src == nil {
		return nil
	} else if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	path := ""
	pkey := keys
	var setv func(any) = nil // 赋值回调
	curr := src
	for _, ikey := range keys {
		if curr == nil {
			return nil
		}
		pkey = pkey[1:]
		switch cur := curr.(type) {
		case map[string]any:
			path, curr, setv = mapIteratorMap(cur, ikey, path, pkey, fpv, vfn)
		case []any:
			path, curr, setv = mapIteratorArr(cur, ikey, path, pkey, fpv, vfn, setv)
		case map[any]any:
			path, curr, setv = mapIteratorMap(cur, ikey, path, pkey, fpv, vfn)
		case []map[any]any:
			path, curr, setv = mapIteratorArr(cur, ikey, path, pkey, fpv, vfn, setv)
		case []map[string]any:
			path, curr, setv = mapIteratorArr(cur, ikey, path, pkey, fpv, vfn, setv)
		default:
			// 其他类型暂不支持
			curr = nil
		}
	}
	return curr
}

func mapIteratorMap[K comparable](cur map[K]any, ikey, path string, pkey []string, fpv bool, vfn func(string, any) (any, int8)) (string, any, func(any)) {
	var mk K
	var ck string
	if mks := FindByFieldInMap(cur, ikey, true); len(mks) > 0 {
		mk = mks[0] // 优先检索
		if ik, ok := any(mk).(string); ok {
			ck = ik
		} else {
			ck = fmt.Sprintf("%v", mk)
		}
	} else if IsMatchFuzzyKey(ikey) {
		return path, nil, nil // 找不到字段
	} else if ik, ok := any(ikey).(K); ok {
		mk = ik // 兼容 string 类型
		ck = ikey
	} else {
		return path, nil, nil // 找不到字段
	}
	// 处理路径
	if strings.IndexByte(ck, '.') > 0 {
		ck = "[" + ck + "]"
	}
	if path != "" {
		ck = path + "." + ck
	}
	path = ck
	// 处理数据
	curr, _ := cur[mk]
	if plen := len(pkey); plen == 0 && vfn != nil {
		if v, r := vfn(path, curr); r > 0 {
			cur[mk] = v
		} else if r < 0 {
			delete(cur, mk)
		}
	} else if curr == nil && fpv && plen > 0 {
		// 未到末尾，已经没有值了， 创建字段
		if next := pkey[0]; next == "-0" {
			curr = []any{}
			cur[mk] = curr // 创建数组
		} else {
			curr = map[string]any{}
			cur[mk] = curr // 创建字段
		}
	}
	var vset func(any) = nil
	if vfn != nil {
		vset = func(v any) { cur[mk] = v }
	}
	return path, curr, vset
}

func mapIteratorArr[T any](cur []T, ikey, path string, pkey []string, fpv bool, vfn func(string, any) (any, int8), setv func(any)) (string, any, func(any)) {
	var curr any = nil
	var vset func(any) = nil
	if ikey == "-0" {
		path += ".-0"
		if vfn != nil {
			// 末尾追加数据
			if plen := len(pkey); plen == 0 {
				if v, r := vfn(path, nil); r > 0 {
					cur = append(cur, v.(T))
					if setv != nil {
						setv(cur)
					}
					i := len(cur) - 1
					vset = func(v any) { cur[i] = v.(T) }
				}
			} else if fpv && plen > 0 {
				// 未到末尾，已经没有值了， 创建字段
				if next := pkey[0]; next == "-0" {
					curr = []any{}
					cur = append(cur, curr.(T)) // 创建数组
					// 这里存在风险，直接多维数组， 必须是 [][][]...any 类型
				} else {
					curr = map[string]any{}
					cur = append(cur, curr.(T)) // 创建字段
				}
				if setv != nil {
					setv(cur)
				}
				ai := len(cur) - 1
				vset = func(v any) { cur[ai] = v.(T) }
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
			path += "." + strconv.Itoa(ai)
			curr = cur[ai]
			if len(pkey) == 0 && vfn != nil {
				if v, r := vfn(path, curr); r > 0 {
					cur[ai] = v.(T) // 更新字段
				} else if r < 0 {
					cur = SliceDelete(cur, ai)
					if setv != nil {
						setv(cur)
					}
				}
			}
			if vfn != nil {
				vset = func(v any) { cur[ai] = v.(T) }
			}
		} else {
			curr = nil
		}
	}
	return path, curr, vset
}

// ---------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------

func Ptr[T any](v T) *T {
	return &v
}

type Pair struct {
	K string
	V any
}

type PairSlice []Pair

// slices.Delete(s, i, i+1)
func SliceDelete[S ~[]E, E any](s S, i int) S {
	if i < 0 && i >= len(s) {
		return nil
	}
	j := i + 1
	oldlen := len(s)
	s = append(s[:i], s[j:]...)
	clear(s[len(s):oldlen]) // zero/nil out the obsolete elements, for GC
	return s
}

// func SliceInsert[S ~[]E, E any](s S, i int, val E) S {
// 	return slices.Insert(s, i, val)
// }

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
// src: 检索的 map, key: 属性字段， oly: 是否只返回一个结果, 比 key = *, ? 随机取一个
func FindByFieldInMap[K comparable](src map[K]any, key string, oly bool) []K {
	ks := []K{}
	if key == "*" || key == "?" {
		// 匹配所有字段, * 所有， ？匹配到1个就返回
		for k := range src {
			ks = append(ks, k)
			if oly {
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
		if v3 != nil && IsMatchMapField(v3, k2[1], kre) {
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
		if v3 != nil && IsMatchMapField(v3, k2[1], kre) {
			ks = append(ks, ck) // 匹配到结果
			if oly {
				break
			}
		}
	}
	return ks
}

func IsMatchFuzzyKey(ikey string) bool {
	if ikey == "*" || ikey == "?" {
		return true // 匹配模式， 忽略
	}
	if ll := len(ikey); ll > 0 && strings.ContainsRune(ikey, '=') && //
		(ikey[0] == '.' || strings.Index(ikey, "[.") > 0 && ikey[ll-1] == ']') {
		return true // .xxx=xxx 开头 或者 x[.xxx=xxx] 格式， 忽略
	}
	return false
}

// 执行内容匹配， 暂时只支持 int, int64, float64, string, bool 类型
func IsMatchMapField(val any, src string, kre *regexp.Regexp) bool {
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
// src: 检索的 数组, key: 属性字段， only: 是否只返回一个结果, 比 key = *, ? 优先级高
func FindByFieldInArr[T any](src []T, key string, oly bool) []int {
	ks := []int{}
	if key == "*" || key == "?" {
		// 匹配所有
		for i := range src {
			ks = append(ks, i)
			if oly {
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
		} else if v2, ok := any(v).(map[string]any); ok {
			v3, _ = v2[k2[0]]
		} else if v2, ok := any(v).(map[any]any); ok {
			v3, _ = v2[k2[0]]
		}
		if v3 != nil && IsMatchMapField(v3, k2[1], kre) {
			// 匹配到结果
			ks = append(ks, i)
			if oly {
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
