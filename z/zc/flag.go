// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package zc

import (
	"flag"
	"strings"
)

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
