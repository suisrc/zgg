// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 默认系统只提供向 tty 发送日志 和 syslog 发送日志
// 对于想使用文件保存日志的，可以重置 Log 完成

package logsyslog

import (
	"io"
	"log/slog"
	"log/syslog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

// 日志 通过 syslog 发送

// 重试间隔
var TryInterval = 5

func init() {
	// 注册初始化Logger方法
	zc.InitFunc = InitSysLog
}

func InitSysLog() {
	if zc.C.Syslog == "" {
		return // 不进行初始化
	}
	addr := zc.C.Syslog
	net := "udp"
	if idx := strings.Index(addr, "://"); idx > 0 {
		net = addr[:idx]
		addr = addr[idx+3:]
	}
	// 创建 syslog.Writer
	writer := NewSyslogWriter(addr, net, 0, zc.C.LogTty).Init()
	switch zc.C.LogTyp {
	case "text":
		logger := slog.New(slog.NewTextHandler(writer, nil))
		slog.SetDefault(logger) // 替换默认日志记录器
	case "json":
		logger := slog.New(slog.NewJSONHandler(writer, nil))
		slog.SetDefault(logger) // 替换默认日志记录器
	default:
		logger := slog.New(zc.NewLogStdHandler(writer, nil))
		slog.SetDefault(logger) // 替换默认日志记录器
	}
}

func NewSyslogWriter(addr, net string, fac int, tty bool) *lSyslog {
	return (&lSyslog{
		Network:  net,
		Address:  addr,
		SyncTty:  tty,
		Priority: syslog.Priority(fac),
	}).Init()
}

type lSyslog struct {
	Network string // udp/tcp
	Address string // 127.0.0.1:5141
	TagInfo string // app.ns， 应用.空间
	SyncTty bool   // 同步终端输出

	Priority syslog.Priority // syslog 优先级，默认 LOG_LOCAL0

	// 由于 udp 协议有掉线的风险，所以每5s重建一个syslog.Writer
	// 其次，Writer 中本身有锁，这里加个锁不会影响业务的实际效果
	// 没什么不用多实例高并发？1.资源成本控制， 2.防止接受日志服务器崩溃
	// 会影响业务性能吗？不会，日志处理本身就是在独立的 goroutine 中执行
	klog *syslog.Writer
	lock sync.Mutex
	unix int64 // time.Unix, 单位是秒
}

func (r *lSyslog) Init() *lSyslog {
	if r.Network == "" {
		r.Network = "udp"
	} else if r.Network != "udp" && r.Network != "tcp" {
		zc.LogTty("[_lsyslog]:", "invalid network,", r.Network)
		return r
	}
	if r.TagInfo == "" {
		r.TagInfo = z.AppName
		ns := zc.GetNamespace()
		if ns != "-" {
			r.TagInfo += "." + ns
		}
	}
	if r.Priority <= 0 {
		r.Priority = syslog.LOG_LOCAL0 | syslog.LOG_INFO
	}
	return r
}

var _ io.Writer = (*lSyslog)(nil)
var _ io.Closer = (*lSyslog)(nil)

func (r *lSyslog) Write(buf []byte) (int, error) {
	blen := len(buf)
	if blen == 0 {
		return 0, nil
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	// 检查 syslog 服务器链接
	if r.klog == nil && r.Address != "" {
		var err error
		r.klog, err = syslog.Dial(r.Network, r.Address, r.Priority, r.TagInfo)
		if err != nil {
			zc.ErrTty("[_lsyslog]:", "unable to connect to syslog:", err.Error())
		} else {
			zc.LogTty("[_lsyslog]:", "connect to syslog:", r.Address)
		}
		r.unix = time.Now().Unix() + int64(TryInterval)
	}
	if r.klog == nil {
		// 降级到终端输出
		if buf[blen-1] == '\n' {
			os.Stdout.Write(buf)
		} else {
			// 正常情况都会带有换行符
			os.Stdout.Write(append(buf, '\n'))
		}
		return blen, nil
	}
	if r.SyncTty {
		// 同步在终端输出
		if buf[blen-1] == '\n' {
			os.Stdout.Write(buf)
		} else {
			// 正常情况都会带有换行符
			os.Stdout.Write(append(buf, '\n'))
		}
	}
	// 发送日志到 syslog 服务器
	if r.unix < time.Now().Unix() {
		// 重置 syslog.Writer
		r.klog.Close()
		r.unix = time.Now().Unix() + int64(TryInterval)
	}
	if err := r.klog.Info(string(buf)); err != nil {
		zc.LogTty("[_lsyslog]:", "unable to write to syslog: ", err.Error())
		// 写出发生异常，可能是连接断开了，重置 syslog.Writer
		r.klog.Close()
		r.klog = nil // 置空，需要重新检查Address等信息， 等待下次重新 Dial 连接
	}
	return blen, nil
}

func (r *lSyslog) Close() error {
	if r.klog != nil {
		return r.klog.Close()
	}
	return nil
}
