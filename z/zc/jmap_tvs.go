package zc

import (
	"fmt"
	"strconv"
	"strings"
)

// 使用循环的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapVal(src any, keys ...string) Pair {
	pairs := MapTraverse(src, 1, keys...) // 遍历栈元素：保存单次处理的上下文
	if len(pairs) == 0 {
		return Pair{}
	}
	return pairs[0]
}

// 使用循环的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapVals(src any, keys ...string) []Pair {
	return MapTraverse(src, -1, keys...) // 遍历栈元素：保存单次处理的上下文
}

// [只读模式], MapRecursion 和 MapTraverse 功能相同。
// 当前只基于 map[string]any, []any, map[any]any, []map[any]any, []map[string]any。 max < 0 获取所有的值。
func MapTraverse(src any, max int, keys ...string) []Pair {
	dest := []Pair{}
	if len(keys) == 0 || src == nil || max == 0 {
		return dest
	} else if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	// 遍历栈元素：保存单次处理的上下文
	bmap := map[string]*bool{}
	type node struct {
		elem any      // 当前处理的对象
		path string   // 当前已拼接的路径
		keys []string // 剩余待匹配的key列表
		only *bool    // 是否需要匹配
	}
	// 初始化栈，放入初始参数
	stack := []*node{{elem: src, path: "", keys: keys}}
	for len(stack) > 0 {
		// 弹出栈顶元素（LIFO，保证遍历顺序与原递归一致）
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if curr.elem == nil || curr.only != nil && *curr.only || len(curr.keys) == 0 && curr.path == "" {
			continue // 不满足处理条件， 跳过结果
		}
		// 没有剩余key，直接存入结果
		if len(curr.keys) == 0 {
			dest = append(dest, Pair{curr.path, curr.elem})
			if max > 0 {
				if max -= 1; max == 0 {
					return dest
				}
			}
			for key, val := range bmap {
				if strings.HasPrefix(curr.path, key) {
					*val = true
				}
			}
			continue // 结束当前层
		}
		ikey := curr.keys[0]
		var only *bool = nil
		if ikey == "?" {
			only = Ptr(false)
			bmap[curr.path+"."] = only
		}
		rkey := curr.keys[1:]
		switch cur := curr.elem.(type) {
		case map[string]any:
			mapTraverseMap(cur, false, curr.path, ikey, func(path string, _ string, val any) {
				stack = append(stack, &node{val, path, rkey, only})
			})
			// mks := FindByFieldInMap(cur, ikey, false)
			// if len(mks) == 0 {
			// 	if IsMatchFuzzyKey(ikey) {
			// 		continue // 匹配模式， 忽略
			// 	} else {
			// 		mks = []string{ikey}
			// 	}
			// }
			// // 倒序遍历保证执行顺序与原递归一致
			// for i := len(mks) - 1; i >= 0; i-- {
			// 	key := mks[i]
			// 	val, exist := cur[key]
			// 	if !exist {
			// 		continue
			// 	}
			// 	pkey, ok := any(key).(string)
			// 	if !ok {
			// 		pkey = fmt.Sprintf("%v", key)
			// 	}
			// 	if strings.IndexByte(pkey, '.') >= 0 {
			// 		pkey = "[" + pkey + "]"
			// 	}
			// 	if curr.path != "" {
			// 		pkey = curr.path + "." + pkey
			// 	}
			// 	// 场景压入栈继续处理
			// 	stack = append(stack, &node{val, pkey, rkey, only})
			// }
		case []any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{val, path, rkey, only})
			})
			// if ikey == "-0" {
			// 	continue
			// }
			// switch {
			// case strings.HasPrefix(ikey, "-"):
			// 	// 负索引倒序检索
			// 	if i, err := strconv.Atoi(ikey[1:]); err == nil && i > 0 && i <= len(cur) {
			// 		idx := len(cur) - i
			// 		val := cur[idx]
			// 		pkey := strconv.Itoa(idx)
			// 		if curr.path != "" {
			// 			pkey = curr.path + "." + pkey
			// 		}
			// 		stack = append(stack, &node{val, pkey, rkey, only})
			// 	}
			// case strings.HasPrefix(ikey, ".") || ikey == "*" || ikey == "?":
			// 	// 按属性检索数组元素
			// 	ais := FindByFieldInArr(cur, ikey, false)
			// 	for i := len(ais) - 1; i >= 0; i-- {
			// 		idx := ais[i]
			// 		val := cur[idx]
			// 		pkey := strconv.Itoa(idx)
			// 		if curr.path != "" {
			// 			pkey = curr.path + "." + pkey
			// 		}
			// 		stack = append(stack, &node{val, pkey, rkey, only})
			// 	}
			// default:
			// 	// 正索引检索
			// 	idx, err := strconv.Atoi(ikey)
			// 	if err == nil && idx >= 0 && idx < len(cur) {
			// 		val := cur[idx]
			// 		pkey := strconv.Itoa(idx)
			// 		if curr.path != "" {
			// 			pkey = curr.path + "." + pkey
			// 		}
			// 		stack = append(stack, &node{val, pkey, rkey, only})
			// 	}
			// }
		case map[any]any:
			mapTraverseMap(cur, false, curr.path, ikey, func(path string, _ any, val any) {
				stack = append(stack, &node{val, path, rkey, only})
			})
		case []map[any]any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{val, path, rkey, only})
			})
		case []map[string]any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{val, path, rkey, only})
			})
		default:
			// ignore
		}
	}
	return dest
}

func mapTraverseMap[K comparable](cur map[K]any, fpv bool, path, ikey string, setv func(string, K, any)) {
	// 查找匹配的key
	mks := FindByFieldInMap(cur, ikey, false)
	if len(mks) == 0 {
		if IsMatchFuzzyKey(ikey) {
			return // 匹配模式， 忽略
		} else if ik, ok := any(ikey).(K); ok {
			mks = []K{ik}
		} else {
			return // 忽略
		}
	}
	// 倒序遍历保证执行顺序与原递归一致
	for i := len(mks) - 1; i >= 0; i-- {
		key := mks[i]
		val, exist := cur[key]
		if !exist && !fpv {
			continue
		}
		pkey, ok := any(key).(string)
		if !ok {
			pkey = fmt.Sprintf("%v", key)
		}
		if strings.IndexByte(pkey, '.') >= 0 {
			pkey = "[" + pkey + "]"
		}
		if path != "" {
			pkey = path + "." + pkey
		}
		// 场景压入栈继续处理
		setv(pkey, key, val)
	}
}

func mapTraverseArr[T any](cur []T, fpv bool, path, ikey string, setv func(string, int, any)) {
	if ikey == "-0" {
		if fpv {
			idx := len(cur)
			pkey := strconv.Itoa(idx)
			if path != "" {
				pkey = path + "." + pkey
			}
			setv(pkey, -1, nil)
		}
		return
	}
	switch {
	case strings.HasPrefix(ikey, "-"):
		// 负索引倒序检索
		if i, err := strconv.Atoi(ikey[1:]); err == nil && i > 0 && i <= len(cur) {
			idx := len(cur) - i
			val := cur[idx]
			pkey := strconv.Itoa(idx)
			if path != "" {
				pkey = path + "." + pkey
			}
			setv(pkey, idx, val)
		}
	case strings.HasPrefix(ikey, ".") || ikey == "*" || ikey == "?":
		// 按属性检索数组元素
		ais := FindByFieldInArr(cur, ikey, false)
		for i := len(ais) - 1; i >= 0; i-- {
			idx := ais[i]
			val := cur[idx]
			pkey := strconv.Itoa(idx)
			if path != "" {
				pkey = path + "." + pkey
			}
			setv(pkey, idx, val)
		}
	default:
		// 正索引检索
		idx, err := strconv.Atoi(ikey)
		if err == nil && idx >= 0 && idx < len(cur) {
			val := cur[idx]
			pkey := strconv.Itoa(idx)
			if path != "" {
				pkey = path + "." + pkey
			}
			setv(pkey, idx, val)
		}
	}
}

// ---------------------------------------------------------------------------------------
// 只读取一个结果， (any 处理值, int8[-1 删除， 0 不变， 1 替换], bool[是否继续变量])

// 使用循环的方式检索一个符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapGet1(src any, keys ...string) Pair {
	pair := Pair{}
	MapTraverseSet(src, false, func(k string, v any) (any, int8, bool) {
		pair.K, pair.V = k, v
		return nil, 0, false
	}, keys...)
	return pair
}

// 使用循环的方式检索一个符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapSet1(src any, val any, keys ...string) Pair {
	pair := Pair{}
	MapTraverseSet(src, true, func(k string, v any) (any, int8, bool) {
		pair.K, pair.V = k, v
		return val, If[int8](val == nil, -1, 1), false
	}, keys...)
	return pair
}

// 使用循环的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapGets(src any, keys ...string) []Pair {
	pairs := PairSlice{}
	MapTraverseSet(src, false, func(k string, v any) (any, int8, bool) {
		pairs = append(pairs, Pair{k, v})
		return nil, 0, true
	}, keys...)
	return pairs
}

// 使用循环的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapSets(src any, val any, keys ...string) []Pair {
	pairs := PairSlice{}
	MapTraverseSet(src, true, func(k string, v any) (any, int8, bool) {
		pairs = append(pairs, Pair{k, v})
		return val, If[int8](val == nil, -1, 1), true
	}, keys...)
	return pairs
}

// [读写模式], 基于循环的方式，对值进行匹配， 也可以使用这个方法进行值获取
func MapTraverseSet(src any, fpv bool, vfn func(string, any) (any, int8, bool), keys ...string) {
	if len(keys) == 0 || vfn == nil || src == nil {
		return
	} else if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	// 遍历栈元素：保存单次处理的上下文
	// 初始化栈，放入初始参数
	stack := []*mapTraverseNode{{elem: src, path: "", keys: keys}}
	has1st := false
	for len(stack) > 0 {
		// 弹出栈顶元素（LIFO，保证遍历顺序与原递归一致）
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !fpv && curr.elem == nil || curr.x1st != nil && *curr.x1st || len(curr.keys) == 0 && curr.path == "" {
			continue // 不满足处理条件， 跳过结果
		}
		// 没有剩余key，直接存入结果
		if len(curr.keys) == 0 {
			value, cover, next := vfn(curr.path, curr.elem)
			if cover > 0 {
				curr.update(value)
			} else if cover < 0 {
				curr.delete()
			}
			if !next {
				return // 强制中断遍历, 返回结果
			}
			if has1st {
				// 通知上层 [1st -> '?'] 模块，内容已经找到
				if curr.x1st != nil {
					*curr.x1st = true
				}
				for curr.from != nil {
					curr = curr.from
					if curr.x1st != nil {
						*curr.x1st = true
					}
				}
			}
			continue // 结束当前层
		}
		ikey := curr.keys[0]
		var x1st *bool = nil
		if ikey == "?" {
			x1st, has1st = Ptr(false), true
		}
		rkey := curr.keys[1:]
		if fpv && curr.elem == nil {
			if ikey == "-0" {
				pkey := If(curr.path == "", "0", curr.path+".0")
				stack = append(stack, &mapTraverseNode{curr, nil, pkey, rkey, x1st, nil, -1})
			} else {
				pkey := If(curr.path == "", ikey, curr.path+"."+ikey)
				stack = append(stack, &mapTraverseNode{curr, nil, pkey, rkey, x1st, ikey, 0})
			}
			continue
		} else if curr.elem == nil {
			continue
		}
		switch cur := curr.elem.(type) {
		case map[string]any:
			mapTraverseMap(cur, fpv, curr.path, ikey, func(path string, key string, val any) {
				stack = append(stack, &mapTraverseNode{curr, val, path, rkey, x1st, key, 0})
			})
		case []any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &mapTraverseNode{curr, val, path, rkey, x1st, nil, idx})
			})
		case map[any]any:
			mapTraverseMap(cur, fpv, curr.path, ikey, func(path string, key any, val any) {
				stack = append(stack, &mapTraverseNode{curr, val, path, rkey, x1st, key, 0})
			})
		case []map[any]any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &mapTraverseNode{curr, val, path, rkey, x1st, nil, idx})
			})
		case []map[string]any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &mapTraverseNode{curr, val, path, rkey, x1st, nil, idx})
			})
		default:
			// ignore
		}
	}
}

type mapTraverseNode struct {
	from *mapTraverseNode
	elem any      // 当前处理的对象
	path string   // 当前已拼接的路径
	keys []string // 剩余待匹配的key列表
	x1st *bool    // 是否匹配到
	ikey any
	iidx int
}

func (aa *mapTraverseNode) update(val any) {
	if aa.from == nil {
		return
	}
	if aa.from.elem == nil {
		if aa.ikey == nil {
			aa.from.elem = []any{}
			aa.from.update(aa.from.elem)
		} else if _, ok := aa.ikey.(string); ok {
			aa.from.elem = map[string]any{}
			aa.from.update(aa.from.elem)
		} else {
			aa.from.elem = []map[any]any{}
			aa.from.update(aa.from.elem)
		}
	}
	switch cur := aa.from.elem.(type) {
	case map[string]any:
		cur[aa.ikey.(string)] = val
	case []any:
		if aa.iidx < 0 {
			aa.from.update(append(cur, val))
		} else {
			cur[aa.iidx] = val
		}
	case map[any]any:
		cur[aa.ikey] = val
	case []map[any]any:
		if val, ok := val.(map[any]any); ok {
			if aa.iidx < 0 {
				aa.from.update(append(cur, val))
			} else {
				cur[aa.iidx] = val
			}
		}
	case []map[string]any:
		if val, ok := val.(map[string]any); ok {
			if aa.iidx < 0 {
				aa.from.update(append(cur, val))
			} else {
				cur[aa.iidx] = val
			}
		}
	default:
		// ignore
	}
}

func (aa *mapTraverseNode) delete() {
	if aa.from == nil {
		return
	}
	switch cur := aa.from.elem.(type) {
	case map[string]any:
		delete(cur, aa.ikey.(string))
	case []any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.update(vv)
		}
	case map[any]any:
		delete(cur, aa.ikey)
	case []map[any]any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.update(vv)
		}
	case []map[string]any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.update(vv)
		}
	default:
		// ignore
	}
}
