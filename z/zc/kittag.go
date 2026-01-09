// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 反射处理标签

package zc

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
)

type Tag struct {
	Keys  []string
	Tags  []string
	Field reflect.StructField
	Value reflect.Value
	Nodes []*Tag // 子字段
}

func ToTagMust(val any, tag string) ([]*Tag, reflect.Kind) {
	return ToTag(val, tag, true, nil)
}

func ToTag(val any, tag string, mst bool, pks []string) ([]*Tag, reflect.Kind) {
	if val == nil {
		return []*Tag{}, reflect.Invalid
	}
	vtype := reflect.TypeOf(val)
	value := reflect.ValueOf(val)
	if vtype.Kind() == reflect.Pointer {
		if value.IsNil() {
			return []*Tag{}, reflect.Invalid
		}

		vtype = vtype.Elem()
		value = value.Elem()
	}
	// if !value.IsValid() {
	// 	return []*Tag{}, vtype.Kind()
	// }
	if pks == nil {
		pks = make([]string, 0)
	}
	// 获取字段标签
	vtags := []*Tag{}
	for i := 0; i < vtype.NumField(); i++ {
		field := vtype.Field(i)
		ftags := []string{}
		if tag != "" {
			// 通过标签字段，获取标签内容
			tagVal := field.Tag.Get(tag)
			if tagVal == "-" {
				continue
			}
			if tagVal != "" {
				// 通过标签字段，获取标签内容
				ftags = strings.Split(tagVal, ",")
			}
		}
		if !mst && len(ftags) == 0 {
			continue
		}
		if len(ftags) == 0 {
			// mst: 未获取到标签内容，强制使用属性字段标记
			ftags = []string{strings.ToLower(field.Name)}
		}
		vtags = append(vtags, &Tag{
			Keys:  append(slices.Clone(pks), ftags[0]),
			Tags:  ftags,
			Field: field,
			Value: value.Field(i),
		})
	}
	return vtags, vtype.Kind()
}

// ToTagMap ...
func ToTagMap(val any, tag string, mst bool, pks []string) ([]*Tag, []*Tag, reflect.Kind) {
	tags, kind := ToTag(val, tag, mst, pks)
	if len(tags) == 0 {
		return tags, tags, kind
	}
	alls := tags[:]
	for _, vtag := range tags {
		fty := vtag.Field.Type
		if fty.Kind() == reflect.Struct {
			// struct
			stag, sall, _ := ToTagMap(vtag.Value.Addr().Interface(), tag, mst, vtag.Keys)
			alls = append(alls, sall...)
			vtag.Nodes = stag
		} else if fty.Kind() == reflect.Pointer && fty.Elem().Kind() == reflect.Struct {
			// *struct
			stag, sall, _ := ToTagMap(vtag.Value.Interface(), tag, mst, vtag.Keys)
			alls = append(alls, sall...)
			vtag.Nodes = stag
		} else if fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Struct {
			// []struct
			vlen := vtag.Value.Len()
			vtag.Nodes = make([]*Tag, vlen)
			for i := range vlen {
				keys := append(slices.Clone(vtag.Keys), strconv.Itoa(i))
				stag, sall, _ := ToTagMap(vtag.Value.Index(i).Addr().Interface(), tag, mst, keys)
				alls = append(alls, sall...)
				vtag.Nodes = append(vtag.Nodes, &Tag{
					Keys: keys,
					Tags: []string{strconv.Itoa(i)},
					Field: reflect.StructField{
						Name:  strconv.Itoa(i),
						Type:  fty, // slice
						Index: []int{i},
					},
					Value: vtag.Value.Index(i),
					Nodes: stag,
				})
			}
		} else if fty.Kind() == reflect.Slice && fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
			// []*struct
			vlen := vtag.Value.Len()
			vtag.Nodes = make([]*Tag, vlen)
			for i := range vlen {
				keys := append(slices.Clone(vtag.Keys), strconv.Itoa(i))
				stag, sall, _ := ToTagMap(vtag.Value.Index(i).Interface(), tag, mst, keys)
				alls = append(alls, sall...)
				vtag.Nodes = append(vtag.Nodes, &Tag{
					Keys: keys,
					Tags: []string{strconv.Itoa(i)},
					Field: reflect.StructField{
						Name:  strconv.Itoa(i),
						Type:  fty, // slice
						Index: []int{i},
					},
					Value: vtag.Value.Index(i),
					Nodes: stag,
				})
			}
		} else if fty.Kind() == reflect.Map && fty.Elem().Kind() == reflect.Struct {
			// map[string]struct
			vtag.Nodes = make([]*Tag, vtag.Value.Len())
			vmap := vtag.Value.MapRange()
			indx := 0
			for vmap.Next() {
				ekey := vmap.Key().Interface().(string)
				keys := append(slices.Clone(vtag.Keys), ekey)
				stag, sall, _ := ToTagMap(vmap.Value().Interface(), tag, mst, keys)
				alls = append(alls, sall...)
				vtag.Nodes = append(vtag.Nodes, &Tag{
					Keys: keys,
					Tags: []string{ekey},
					Field: reflect.StructField{
						Name:  ekey,
						Type:  fty, // map
						Index: []int{indx},
					},
					Value: vmap.Value(),
					Nodes: stag,
				})
				indx += 1
			}
		} else if fty.Kind() == reflect.Map && fty.Elem().Kind() == reflect.Pointer && fty.Elem().Elem().Kind() == reflect.Struct {
			// map[string]*struct
			vtag.Nodes = make([]*Tag, vtag.Value.Len())
			vmap := vtag.Value.MapRange()
			indx := 0
			for vmap.Next() {
				ekey := vmap.Key().Interface().(string)
				keys := append(slices.Clone(vtag.Keys), ekey)
				stag, sall, _ := ToTagMap(vmap.Value().Interface(), tag, mst, keys)
				alls = append(alls, sall...)
				vtag.Nodes = append(vtag.Nodes, &Tag{
					Keys: keys,
					Tags: []string{ekey},
					Field: reflect.StructField{
						Name:  ekey,
						Type:  fty, // map
						Index: []int{indx},
					},
					Value: vmap.Value(),
					Nodes: stag,
				})
				indx += 1
			}
		} else {
			// switch fty.Kind() {
			// case reflect.Bool, reflect.String:
			// 	vtag.Basic = true
			// case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// 	vtag.Basic = true
			// case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			// 	vtag.Basic = true
			// case reflect.Float32, reflect.Float64:
			// 	vtag.Basic = true
			// }
		}
	}
	return tags, alls, kind
}

func ToTagVal(val any, tag string) []*Tag {
	_, alls, _ := ToTagMap(val, tag, true, nil)
	tbs := []*Tag{}
	for _, tag := range alls {
		if len(tag.Nodes) == 0 {
			// if tag.Nodes == nil {
			tbs = append(tbs, tag)
		}
	}
	return tbs
}
