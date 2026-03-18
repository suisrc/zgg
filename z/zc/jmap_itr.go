package zc

// 这是第一个版本的 MAP 检索器
// MapIterator 读写器
// 不支持 * 和 ? 匹配, 只起到占位符的作用，不会进行遍历，只会遍历一条路径， 如果存在 * 和 ? 或者 [.name=^re] 不保证路径一定有效

import (
	"fmt"
	"strconv"
	"strings"
)

func MapGet1V1(src any, keys ...string) any {
	if src == nil {
		return Pair{}
	}
	if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	return MapIterator(src, false, nil, keys...)
}

func MapSet1V1(src any, val any, keys ...string) any {
	if src == nil {
		return nil
	}
	if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	return MapIterator(src, false, func(_ string, _ any) (any, int8) {
		return val, If[int8](val == nil, -1, 1)
	}, keys...)
}

// [读写模式], 使用循环的方式检索所有符合条件的内容
// vfn (any 处理值, int8[-1 删除， 0 不变， 1 替换])
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
