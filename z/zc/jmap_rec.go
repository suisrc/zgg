package zc

import (
	"strconv"
	"strings"
)

// 检索属于规范的 key 列表和对应的值
func MapVars(src any, keys ...string) []Pair {
	if src == nil {
		return []Pair{}
	}
	if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	return MapRecursion(src, "", keys...)
}

// 检索属于规范的 key 列表和对应的值，Iterator or Traverse or Recursion
// MapRecursion 和 MapTraverse 功能相同。基于测试， MapTraverse 效率会更好一些， 百万次查询，相差30%左右。
// 当前只基于 map[string]any 和 []any 进行处理。推荐使用 MapTraverse
func MapRecursion(curr any, path string, keys ...string) []Pair {
	if curr == nil || len(keys) == 0 && path == "" {
		return []Pair{}
	}
	if len(keys) == 0 {
		return []Pair{{path, curr}}
	}
	dest := []Pair{} // 返回值列表
	ikey := keys[0]
	keys = keys[1:]
	x1st := ikey == "?"
	switch curr := curr.(type) {
	case map[string]any:
		mks := FindByFieldInMap(curr, ikey, false)
		if len(mks) == 0 {
			if !IsMatchFuzzyKey(ikey) {
				mks = []string{ikey} // 使用默认值到 key
			}
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
				dst := MapRecursion(cur, key, keys...)
				if x1st && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				dest = append(dest, dst...)
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
				dst := MapRecursion(cur, key, keys...)
				if x1st && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				dest = append(dest, dst...)
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
				dst := MapRecursion(cur, key, keys...)
				if x1st && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				dest = append(dest, dst...)
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
				dst := MapRecursion(cur, key, keys...)
				if x1st && len(dst) > 0 {
					return dst // 找到一个就返回
				}
				dest = append(dest, dst...)
			}
		}
	}
	return dest
}
