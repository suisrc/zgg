package zc

import (
	"fmt"
	"strconv"
	"strings"
)

// 检索属于规范的 key 列表和对应的值，返回 []Pair
func MapVals(src any, keys ...string) []Pair {
	rst := PairSlice{}
	MapTraverse(src, rst.Add, keys...) // 遍历栈元素：保存单次处理的上下文
	return rst
}

// 检索属于规范的 key 列表和对应的值，返回 any, Iterator or Traverse or Recursion
// MapRecursion 和 MapTraverse 功能相同。基于测试， MapTraverse 效率会更好一些， 百万次查询，相差30%左右。
// vfn = nil: 只取一个， 否则通过 vfn 返回所有，而当前函数返回最后一个值
func MapTraverse(src any, vfn func(string, any) bool, keys ...string) any {
	var dest any = nil
	if len(keys) == 0 || src == nil {
		return dest
	} else if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	// 遍历栈元素：保存单次处理的上下文
	type node struct {
		from *node
		elem any      // 当前处理的对象
		path string   // 当前已拼接的路径
		keys []string // 剩余待匹配的key列表
		x1st *bool    // 是否匹配到
	}
	// 初始化栈，放入初始参数
	stack := []*node{{elem: src, path: "", keys: keys}}
	has1st := false
	for len(stack) > 0 {
		// 弹出栈顶元素（LIFO，保证遍历顺序与原递归一致）
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if curr.elem == nil || curr.x1st != nil && *curr.x1st || len(curr.keys) == 0 && curr.path == "" {
			continue // 不满足处理条件， 跳过结果
		}
		// 没有剩余key，直接存入结果
		if len(curr.keys) == 0 {
			dest = curr.elem
			if vfn == nil {
				return dest // vfn 为空只处理一个结果
			}
			if next := vfn(curr.path, curr.elem); !next {
				return dest // 强制中断遍历, 返回结果
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
		switch cur := curr.elem.(type) {
		case map[string]any:
			mapTraverseMap(cur, false, curr.path, ikey, func(path string, _ string, val any) {
				stack = append(stack, &node{curr, val, path, rkey, x1st})
			})
		case []any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{curr, val, path, rkey, x1st})
			})
		case map[any]any:
			mapTraverseMap(cur, false, curr.path, ikey, func(path string, _ any, val any) {
				stack = append(stack, &node{curr, val, path, rkey, x1st})
			})
		case []map[any]any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{curr, val, path, rkey, x1st})
			})
		case []map[string]any:
			mapTraverseArr(cur, false, curr.path, ikey, func(path string, _ int, val any) {
				stack = append(stack, &node{curr, val, path, rkey, x1st})
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
		}
		if ik, ok := any(ikey).(K); ok {
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

func MapSet1(src any, fpv bool, val any, keys ...string) Pair {
	pair := Pair{V: val}
	MapTraverseSet(src, fpv, pair.Set1, keys...)
	return pair
}

func MapSets(src any, fpv bool, val any, keys ...string) []Pair {
	pairs := PairSlice{{V: val}}
	MapTraverseSet(src, fpv, pairs.Sets, keys...)
	return pairs[1:]
}

// 检索属于规范的 key 列表和对应的值，返回 any, Iterator or Traverse or Recursion
func MapTraverseSet(src any, fpv bool, vfn func(string, any) (any, int8, bool), keys ...string) {
	if len(keys) == 0 || vfn == nil || src == nil {
		return
	} else if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	// 遍历栈元素：保存单次处理的上下文
	// 初始化栈，放入初始参数
	stack := []*map_node{{elem: src, path: "", keys: keys}}
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
				curr.set(value)
			} else if cover < 0 {
				curr.del()
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
				stack = append(stack, &map_node{curr, nil, pkey, rkey, x1st, nil, -1})
			} else {
				pkey := If(curr.path == "", ikey, curr.path+"."+ikey)
				stack = append(stack, &map_node{curr, nil, pkey, rkey, x1st, ikey, 0})
			}
			continue
		} else if curr.elem == nil {
			continue
		}
		switch cur := curr.elem.(type) {
		case map[string]any:
			mapTraverseMap(cur, fpv, curr.path, ikey, func(path string, key string, val any) {
				stack = append(stack, &map_node{curr, val, path, rkey, x1st, key, 0})
			})
		case []any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &map_node{curr, val, path, rkey, x1st, nil, idx})
			})
		case map[any]any:
			mapTraverseMap(cur, fpv, curr.path, ikey, func(path string, key any, val any) {
				stack = append(stack, &map_node{curr, val, path, rkey, x1st, key, 0})
			})
		case []map[any]any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &map_node{curr, val, path, rkey, x1st, nil, idx})
			})
		case []map[string]any:
			mapTraverseArr(cur, fpv, curr.path, ikey, func(path string, idx int, val any) {
				stack = append(stack, &map_node{curr, val, path, rkey, x1st, nil, idx})
			})
		default:
			// ignore
		}
	}
}

type map_node struct {
	from *map_node
	elem any      // 当前处理的对象
	path string   // 当前已拼接的路径
	keys []string // 剩余待匹配的key列表
	x1st *bool    // 是否匹配到
	ikey any
	iidx int
}

func (aa *map_node) set(val any) {
	if aa.from == nil {
		return
	}
	if aa.from.elem == nil {
		if aa.ikey == nil {
			aa.from.elem = []any{}
			aa.from.set(aa.from.elem)
		} else if _, ok := aa.ikey.(string); ok {
			aa.from.elem = map[string]any{}
			aa.from.set(aa.from.elem)
		} else {
			aa.from.elem = []map[any]any{}
			aa.from.set(aa.from.elem)
		}
	}
	switch cur := aa.from.elem.(type) {
	case map[string]any:
		cur[aa.ikey.(string)] = val
	case []any:
		if aa.iidx < 0 {
			aa.from.set(append(cur, val))
		} else {
			cur[aa.iidx] = val
		}
	case map[any]any:
		cur[aa.ikey] = val
	case []map[any]any:
		if val, ok := val.(map[any]any); ok {
			if aa.iidx < 0 {
				aa.from.set(append(cur, val))
			} else {
				cur[aa.iidx] = val
			}
		}
	case []map[string]any:
		if val, ok := val.(map[string]any); ok {
			if aa.iidx < 0 {
				aa.from.set(append(cur, val))
			} else {
				cur[aa.iidx] = val
			}
		}
	default:
		// ignore
	}
}

func (aa *map_node) del() {
	if aa.from == nil {
		return
	}
	switch cur := aa.from.elem.(type) {
	case map[string]any:
		delete(cur, aa.ikey.(string))
	case []any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.set(vv)
		}
	case map[any]any:
		delete(cur, aa.ikey)
	case []map[any]any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.set(vv)
		}
	case []map[string]any:
		if vv := SliceDelete(cur, aa.iidx); vv != nil {
			aa.from.set(vv)
		}
	default:
		// ignore
	}
}
