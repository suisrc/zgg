// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// flag 绑定的类型的扩展

package zc

import (
	"flag"
	"strconv"
	"strings"
)

var _ flag.Value = (*stringVal)(nil)

// -- string Value
type stringVal string

func NewStrVal(p *string, v string) *stringVal {
	if *p == "" {
		*p = v
	}
	return (*stringVal)(p)
}

func (s *stringVal) Set(val string) error {
	*s = stringVal(val)
	return nil
}

func (s *stringVal) Get() any { return string(*s) }

func (s *stringVal) String() string { return string(*s) }

// -----------------------------------------------------

// -- bool Value
type boolVal bool

func NewBoolVal(p *bool) *boolVal {
	return (*boolVal)(p)
}

func (b *boolVal) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b = boolVal(v)
	return err
}

func (b *boolVal) Get() any { return bool(*b) }

func (b *boolVal) String() string { return strconv.FormatBool(bool(*b)) }

func (b *boolVal) IsBoolFlag() bool { return true }

// -----------------------------------------------------

// -- int Value
type intVal int

func NewIntVal(p *int) *intVal {
	return (*intVal)(p)
}

func (i *intVal) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, strconv.IntSize)
	*i = intVal(v)
	return err
}

func (i *intVal) Get() any { return int(*i) }

func (i *intVal) String() string { return strconv.Itoa(int(*i)) }

// -----------------------------------------------------

var _ flag.Value = (*StrArr)(nil)

type StrArr []string

func (aa *StrArr) Set(value string) error {
	if value != "" {
		*aa = strings.Split(value, ",")
	}
	return nil
}

func (aa *StrArr) String() string {
	return strings.Join(*aa, ",")
}

func NewStrArr(p *[]string, val []string) *StrArr {
	*p = val
	return (*StrArr)(p)
}

// -----------------------------------------------------

var _ flag.Value = (*StrMap)(nil)

type StrMap map[string]string

func (aa *StrMap) Set(value string) error {
	if value != "" {
		for vv := range strings.SplitSeq(value, ",") {
			kv := strings.SplitN(vv, "=", 2)
			if len(kv) == 2 {
				(*aa)[kv[0]] = kv[1]
			} else {
				(*aa)[kv[0]] = ""
			}
		}
	}
	return nil
}

func (aa *StrMap) String() string {
	var str string
	for k, v := range *aa {
		str += "," + k + "=" + v
	}
	if str != "" {
		str = str[1:]
	}
	return str
}

func NewStrMap(p *map[string]string, val map[string]string) *StrMap {
	*p = val
	return (*StrMap)(p)
}
