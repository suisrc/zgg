// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gtw"
)

// 日志转存到控制台上

func NewRecordPrint() gtw.RecordPool {
	return gtw.NewRecordPool(RecordPrint)
}

func RecordPrint(rt gtw.RecordTrace) {
	rc := &Record{}
	rc.ByRecord0(rt.(*gtw.Record0))
	zc.Println("[_record_]", rc.ToFormatStr())
}

// -----------------------------------
// 日志转存到文件, 简单参考，不要用于生产

func NewRecordSimpleFile(file string) gtw.RecordPool {
	return gtw.NewRecordPool((&rSimpleFile{file: file}).Init().log)
}

type rSimpleFile struct {
	lock sync.Mutex
	file string
}

func (r *rSimpleFile) Init() *rSimpleFile {
	// file 的文件夹是否存在，不存在， 创建文件夹
	_, err := os.Stat(r.file)
	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(r.file), 0644)
	}
	return r
}

func (r *rSimpleFile) log(rt gtw.RecordTrace) {
	rc := &Record{}
	rc.ByRecord0(rt.(*gtw.Record0))
	bs, _ := rc.MarshalJSON()
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
