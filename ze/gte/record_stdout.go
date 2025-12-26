package gte

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/ze/gtw"
)

// 日志转存到控制台上

func NewRecordStdout() gtw.RecordPool {
	return gtw.NewRecordPool(RecordToStdout)
}

func RecordToStdout(rt gtw.RecordTrace) {
	rc := &Record{}
	rc.ByRecord0(rt.(*gtw.Record0))
	bs, _ := rc.MarshalJSON()
	z.Println(string(bs))
}

// 日志转存到文件, 简单参考，不要用于生产

func NewRecordStdoutAndFile(file string) gtw.RecordPool {
	return gtw.NewRecordPool((&recordSimpleFile{file: file}).Init().log)
}

type recordSimpleFile struct {
	lock sync.Mutex
	file string
}

func (r *recordSimpleFile) Init() *recordSimpleFile {
	// file 的文件夹是否存在，不存在， 创建文件夹
	_, err := os.Stat(r.file)
	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(r.file), 0644)
	}
	return r
}

func (r *recordSimpleFile) log(rt gtw.RecordTrace) {
	rc := &Record{}
	rc.ByRecord0(rt.(*gtw.Record0))
	bs, _ := rc.MarshalJSON()
	// z.Println(string(bs))
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
