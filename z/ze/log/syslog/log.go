// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 默认系统只提供向 tty 发送日志 和 syslog 发送日志
// 对于想使用文件保存日志的，可以重置 Log 完成

package logsyslog

import (
	"log/syslog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

// 日志 通过 syslog 发送

func init() {
	// 注册初始化Logger方法
	zc.InitLoggerFn = InitLoggerBySysLog
}

func InitLoggerBySysLog() {
	if zc.C.Syslog == "" {
		return // 不进行初始化
	}
	addr := zc.C.Syslog
	net := "udp"
	if idx := strings.Index(addr, "://"); idx > 0 {
		net = addr[:idx]
		addr = addr[idx+3:]
	}
	zc.Log = NewLoggerSyslog(addr, net, 0, zc.C.LogTty)

}

func NewLoggerSyslog(addr, net string, pir int, tty bool) zc.Logger {
	return (&lSyslog{
		Network:  net,
		Address:  addr,
		Priority: pir,
		PrintTty: tty,
		_pool:    sync.Pool{New: func() any { return new([]byte) }},
	}).Init()
}

type lSyslog struct {
	Network  string // udp/tcp
	Address  string // 127.0.0.1:5141
	Priority int    // 128 ?
	TagInfo  string // app.ns， 应用.空间
	PrintTty bool   // 同步终端输出

	// 由于 udp 协议有掉线的风险，所以每5s重建一个syslog.Writer
	// 其次，Writer 中本身有锁，这里加个锁不会影响业务的实际效果
	// 没什么不用多实例高并发？1.资源成本控制， 2.防止接受日志服务器崩溃
	// 会影响业务性能吗？不会，日志处理本身就是在独立的 goroutine 中执行
	_klog *syslog.Writer
	_lock sync.Mutex
	_time int64 // time.Unix, 单位是秒
	_pool sync.Pool
}

func (r *lSyslog) Init() *lSyslog {
	if r.Network == "" {
		r.Network = "udp"
	} else if r.Network != "udp" && r.Network != "tcp" {
		zc.Printl0("[_lsyslog]:", "invalid network,", r.Network)
		return r
	}
	if r.Address == "" {
		return r // 忽略日志远程输出
	}
	if r.Priority <= 0 {
		r.Priority = int(syslog.LOG_LOCAL0 | syslog.LOG_INFO)
	}
	if r.TagInfo == "" {
		r.TagInfo = z.AppName
		ns := zc.GetNamespace()
		if ns != "-" {
			r.TagInfo += "." + ns
		}
	}
	if r.Address != "" {
		var err error
		r._klog, err = syslog.Dial(r.Network, r.Address, syslog.Priority(r.Priority), r.TagInfo)
		if err != nil {
			zc.Printl0("[_lsyslog]:", "unable to connect to syslog:", err.Error())
		} else {
			zc.Printl0("[_lsyslog]:", "connect to syslog:", r.Address)
		}
		r._time = time.Now().Unix()
	}
	return r
}

func (r *lSyslog) GetBuffer() *[]byte {
	return r._pool.Get().(*[]byte)
}

func (r *lSyslog) PutBuffer(buf *[]byte) {
	// See https://go.dev/issue/23199
	if cap(*buf) > 64<<10 {
		*buf = nil
	}
	*buf = (*buf)[:0]
	r._pool.Put(buf)
}

func (r *lSyslog) Output(depth int, appbuf func([]byte) []byte) error {
	go r._output(depth+1, appbuf)
	return nil
}

func (r *lSyslog) _output(depth int, appbuf func([]byte) []byte) error {
	buf := r.GetBuffer()
	defer r.PutBuffer(buf)

	*buf = appbuf(*buf)
	for i := len(*buf) - 1; i >= 0 && (*buf)[i] == '\n'; i-- {
		*buf = (*buf)[:i]
	}

	if r._klog == nil {
		zc.Printl0(string(*buf))
		return nil // 降级到终端输出
	}
	if r.PrintTty {
		zc.Printl0(string(*buf))
		// 同步在终端输出
	}

	msg := ""
	if zc.LogTrackFile && depth > 0 {
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
		msg = file + ":" + strconv.Itoa(line) + "] "
	}
	msg += string(*buf)

	r._lock.Lock()
	defer r._lock.Unlock()
	if r._time < time.Now().Unix() {
		r._klog.Close() // 重置 syslog.Writer
		r._time = time.Now().Unix() + 5
	}
	if err := r._klog.Info(msg); err != nil {
		zc.Printl0("[_lsyslog]:", "unable to write to syslog: ", err.Error())
	}
	return nil
}
