package zc

// 这是第一个版本的 MAP 检索器
// MapRecursion 只读器
// 当前只基于 map[string]any 和 []any 进行处理， 这仅仅是一个非标准检索器
// 由于递归不如循环检索性能高。它仅是为MapIterator提供补充的多结果过渡方案
// 当前只基于 map[string]any 和 []any 进行处理。

import (
	"strconv"
	"strings"
)

// 使用递归的方式检索所有符合条件的内容， 支持 xxx.-1.*.?.[.name=^re].k[.name=^re].name
func MapGetsV2(src any, keys ...string) []Pair {
	if src == nil {
		return []Pair{}
	}
	if len(keys) == 1 && strings.ContainsRune(keys[0], '.') {
		keys = MapParserPaths(keys[0])
	}
	return MapRecursion(src, "", keys...)
}

// [只读模式], MapRecursion 和 MapTraverse 功能相同。
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
