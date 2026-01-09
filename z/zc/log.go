// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 日志处理

package zc

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	Std = NewLogger(os.Stdout)
	Log = Std

	LogTrackFile = false
	InitLogFunc  = func() {}
)

func Printl0(v ...any) {
	Std.Output(2, func(b []byte) []byte { return fmt.Appendln(b, v...) })
}

func Printf1(format string, v ...any) {
	Log.Output(2, func(b []byte) []byte { return fmt.Appendf(b, format, v...) })
}

// ----------------------------------------------------------------------------

type Logger interface {
	Output(depth int, append func([]byte) []byte) error
}

func NewLogger(w io.Writer) Logger {
	logger := &logger0{}
	logger._pool.New = func() any { return new([]byte) }
	logger._klog = w
	return logger
}

type logger0 struct {
	_pool sync.Pool
	_lock sync.Mutex
	_klog io.Writer // destination for output
}

func (log *logger0) GetBuffer() *[]byte {
	return log._pool.Get().(*[]byte)
}

func (log *logger0) PutBuffer(buf *[]byte) {
	// See https://go.dev/issue/23199
	if cap(*buf) > 64<<10 {
		*buf = nil
	}
	*buf = (*buf)[:0]
	log._pool.Put(buf)
}

func (log *logger0) Output(depth int, appbuf func([]byte) []byte) error {
	now := time.Now() // get this early.

	buf := log.GetBuffer()
	defer log.PutBuffer(buf)

	year, month, day := now.Date()
	LogItoa(buf, year, 4)
	*buf = append(*buf, '-')
	LogItoa(buf, int(month), 2)
	*buf = append(*buf, '-')
	LogItoa(buf, day, 2)
	*buf = append(*buf, ' ')
	hour, min, sec := now.Clock()
	LogItoa(buf, hour, 2)
	*buf = append(*buf, ':')
	LogItoa(buf, min, 2)
	*buf = append(*buf, ':')
	LogItoa(buf, sec, 2)
	*buf = append(*buf, '.')
	LogItoa(buf, now.Nanosecond()/1e3, 6)

	if LogTrackFile && depth > 0 {
		*buf = append(*buf, ' ')
		_, file, line, ok := runtime.Caller(depth)
		if !ok {
			file = "???"
			line = 1
		} else {
			if slash := strings.LastIndex(file, "/"); slash >= 0 {
				path := file
				file = path[slash+1:]
				if dirsep := strings.LastIndex(path[:slash], "/"); dirsep >= 0 {
					file = path[dirsep+1:]
				}
			}
		}
		*buf = append(*buf, file...)
		*buf = append(*buf, ':')
		LogItoa(buf, line, -1)
	}
	*buf = append(*buf, ']', ' ')
	*buf = appbuf(*buf)

	if len(*buf) == 0 || (*buf)[len(*buf)-1] != '\n' {
		*buf = append(*buf, '\n')
	}

	log._lock.Lock()
	defer log._lock.Unlock()
	_, err := log._klog.Write(*buf)
	return err
}

// ----------------------------------------------------------------------------

func LogItoa(buf *[]byte, i int, wid int) {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}
