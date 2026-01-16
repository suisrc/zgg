// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gtw"
)

// 日志转存到控制台上

func NewRecordToTTY(convert gtw.ConvertFunc) gtw.RecordPool {
	return gtw.NewRecordPool(func(record gtw.IRecord) {
		z.Println(convert(record).ToFmt())
	})
}

// -----------------------------------
// 日志转存到文件, 简单参考，不要用于生产

func NewRecordSimpleFile(file string, convert gtw.ConvertFunc) gtw.RecordPool {
	return gtw.NewRecordPool((&rSimpleFile{file: file}).Init().log)
}

type rSimpleFile struct {
	convert gtw.ConvertFunc
	lock    sync.Mutex
	file    string
}

func (r *rSimpleFile) Init() *rSimpleFile {
	// file 的文件夹是否存在，不存在， 创建文件夹
	_, err := os.Stat(r.file)
	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(r.file), 0644)
	}
	return r
}

func (r *rSimpleFile) log(rt gtw.IRecord) {
	bs, _ := r.convert(rt).ToJson()
	r.lock.Lock()
	defer r.lock.Unlock()
	// 追加写入文件
	bs = append(bs, '\n')
	file, err := os.OpenFile(r.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		file.Write(bs)
		defer file.Close()
	}
}
