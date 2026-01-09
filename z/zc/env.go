// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 通过环境遍历初始化配置参数
// 通过TAG标签处理格式化参数

package zc

import (
	"os"
	"reflect"
	"strings"
)

// ENV 解析结果的根结构
type ENV struct {
	Prefix string
}

// 新建 ENV 解析器
func NewENV(prefix string) *ENV {
	return &ENV{Prefix: prefix}
}

// 解析 ENV 数据
func (aa *ENV) Load(val any) error {
	return aa.Decode(val, CFG_TAG)
}

// 解析 ENV 数据
func (aa *ENV) Decode(val any, tag string) error {
	tags := ToTagVal(val, tag)
	for _, tag := range tags {
		// log.Println(tag.Keys)

		key := strings.Join(tag.Keys, "_")
		key = strings.ToUpper(aa.Prefix + "_" + key)
		venv := os.Getenv(key)
		if val, err := StrToBV(tag.Field.Type, venv); err == nil {
			tag.Value.Set(reflect.ValueOf(val))
		}
	}
	return nil
}

// ----------------------------------------------------

// TAG 解析结果的根结构
type TAG struct {
}

// 新建 TAG 解析器
func NewTAG() *TAG {
	return &TAG{}
}

// 解析 TAG 数据
func (aa *TAG) Load(val any) error {
	return aa.Decode(val, CFG_TAG)
}

// 解析 TAG 数据
func (aa *TAG) Decode(val any, tag string) error {
	tags := ToTagVal(val, tag)
	for _, tag := range tags {
		if tag.Value.IsValid() {
			continue
		}
		// 使用默认值进行初始化
		vtag := tag.Field.Tag.Get("default")
		if val, err := StrToBV(tag.Field.Type, vtag); err == nil {
			tag.Value.Set(reflect.ValueOf(val))
		}
	}
	return nil
}
