// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"log/syslog"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gtw"
)

// 日志 通过 syslog 发送

func NewRecordSyslog(addr, net string, pir int, tty bool) gtw.RecordPool {
	return gtw.NewRecordPool((&rSyslog{
		Network:  net,
		Address:  addr,
		Priority: pir,
		PrintTty: tty,
	}).Init().log)
}

type rSyslog struct {
	Network  string // udp/tcp
	Address  string // 127.0.0.1:5141
	Priority int    // 128 ?
	TagInfo  string // app.ns， 应用.空间
	PrintTty bool   // 同步终端输出

	_klog *syslog.Writer
	_lock sync.Mutex
	_time int64 // time.Unix, 单位是秒
}

func (r *rSyslog) Init() *rSyslog {
	if r.Network == "" {
		r.Network = "udp"
	} else if r.Network != "udp" && r.Network != "tcp" {
		zc.Println("[_syslog_]", "rSyslog, invalid network, ", r.Network)
		return r
	}
	if r.Address == "" {
		zc.Println("[_syslog_]", "rSyslog, invalid address, ", r.Address)
		return r
	}
	if r.Priority <= 0 {
		r.Priority = int(syslog.LOG_LOCAL0)
	}
	if r.TagInfo == "" {
		r.TagInfo = z.AppName
		ns := gtw.GetNamespace()
		if ns != "-" {
			r.TagInfo += "." + ns
		}
	}
	if r.Address != "" {
		var err error
		r._klog, err = syslog.Dial(r.Network, r.Address, syslog.Priority(r.Priority), r.TagInfo)
		if err != nil {
			zc.Println("[_syslog_]", "rSyslog, unable to connect to syslog: ", err.Error())
		} else {
			zc.Println("[_syslog_]", "rSyslog, connect to syslog: ", r.Address)
		}
		r._time = time.Now().Unix()
	}
	return r
}

func (r *rSyslog) log(rt gtw.RecordTrace) {
	rc := &Record{}
	rc.ByRecord0(rt.(*gtw.Record0))
	bts, err := rc.MarshalJSON()
	if err != nil {
		zc.Println("[_syslog_]", "rSyslog, unable to marshal json: ", err.Error())
		return
	}
	if r._klog == nil {
		zc.Println("[_record_]", string(bts))
		return // 降级到终端输出
	}
	if r.PrintTty {
		zc.Println("[_syslog_]", string(bts))
		// 同步在终端输出
	}

	r._lock.Lock()
	defer r._lock.Unlock()
	if r._time < time.Now().Unix() {
		// 由于 udp 协议有掉线的风险，所以每 5s 重置 syslog.Writer
		// 其次，Writer 中本身有锁，所以这里即使没有锁也是线程安全的
		// 为什么不用多实例高并发？1.资源成本控制， 2.防止接受日志服务器崩溃
		// 会影响业务性能吗？不会，日志处理本身就是在独立的 goroutine 中执行
		// 日志传送可能存在5s空白期，但是这个极端情况，况且是日志服务器崩溃的情况下，可以忽略
		r._klog.Close() // 重置 syslog.Writer
		r._time = time.Now().Unix() + 5
	}
	if err := r._klog.Info(string(bts)); err != nil {
		zc.Println("[_syslog_]", "rSyslog, unable to write to syslog: ", err.Error())
	}
}

// func (r *rSyslog) all(msg string) {
// 	r.writer.Crit(msg)    // 紧急
// 	r.writer.Err(msg)     // 错误
// 	r.writer.Warning(msg) // 警告
// 	r.writer.Info(msg)    // 信息
// 	r.writer.Debug(msg)   // 调试
// }
