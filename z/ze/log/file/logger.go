// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package logfile

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

func init() {
	// 注册初始化Logger方法
	zc.LS["file"] = InitAppLog
}

func InitAppLog() {
	// 创建 syslog.Writer
	writer := NewWriter(zc.C.Logger.Folder, 0, zc.C.Logger.Tty)
	switch zc.C.Logger.Type {
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

func NewWriter(absPath string, maxSize int64, ttySync bool) io.Writer {
	return &lAppLog{Writer: RollingFile{AbsPath: absPath, MaxSize: maxSize}, TtySync: ttySync}
}

type lAppLog struct {
	Writer  RollingFile
	TtySync bool
}

func (aa *lAppLog) Write(buf []byte) (int, error) {
	blen := len(buf)
	if blen == 0 {
		return 0, nil
	}
	if aa.TtySync {
		// 同步在终端输出
		if buf[blen-1] == '\n' {
			os.Stdout.Write(buf)
		} else {
			// 正常情况都会带有换行符
			os.Stdout.Write(append(buf, '\n'))
		}
	}
	wlen, err := aa.Writer.Write(buf)
	if err != nil {
		zc.LogTty("[_logfile]: unable to write to logfile,", err.Error())
	}
	return wlen, nil
}

func (aa *lAppLog) Close() error {
	return aa.Writer.Close()
}

// ------------------------------------------------------------------------------------

type RollingFile struct {
	CloseFunc func(*RollingFile) // 关闭回调函数， 由外部提供， 以便于回收

	MaxSize int64  // 文件大小限制， 默认 10MB
	AbsPath string // 根路径
	FileKey string // 文件键
	FileHdl *os.File

	Index int         // 文件索引
	fpkey string      // 文件前缀
	flock sync.Mutex  // 写入锁定
	timer *time.Timer // 是否存在
	alive int64       // 存活时间
}

func (aa *RollingFile) Writex(bts ...[]byte) (int, error) {
	if aa.MaxSize <= 0 {
		aa.MaxSize = 10 * 1024 * 1024 // 默认10MB
	}
	if aa.AbsPath == "" {
		aa.AbsPath = zc.C.Logger.Folder // 默认路径
	}
	var fpkey string
	if aa.FileKey != "" {
		// 使用固定的 file key
		fpkey = filepath.Join(aa.AbsPath, aa.FileKey)
	} else if aa.FileKey == "" {
		// 未指定 file key，使用日期作为文件键
		date := time.Now() // %Y/%M/%Y-%M-%D_0.txt
		fkey := fmt.Sprintf("%02d/%02d/%s_", date.Year(), date.Month(), date.Format(time.DateOnly))
		fpkey = filepath.Join(aa.AbsPath, fkey)
	}
	// 由于操作文件句柄，同步锁
	aa.flock.Lock()
	defer aa.flock.Unlock()
	if aa.fpkey != fpkey {
		// 文件键变化，重置索引
		if aa.FileHdl != nil {
			// 关闭当前文件句柄，重置索引
			if aa.CloseFunc != nil {
				aa.CloseFunc(aa)
			}
			if aa.timer != nil {
				aa.timer.Stop()
				aa.timer = nil
			}
			aa.FileHdl.Close()
			aa.FileHdl = nil
		}
		aa.Index = 1 // 文件键变化，重置索引
		aa.fpkey = fpkey
	}
	// defer aa.close()
	if aa.FileHdl != nil {
		if fstat, _ := aa.FileHdl.Stat(); fstat != nil && fstat.Size() > aa.MaxSize {
			// 文件大小超过限制， 关闭文件句柄
			aa.FileHdl.Close()
			aa.FileHdl = nil
			aa.Index++
		} else {
			// 复用文件句柄，写入文件
			defer aa._check()
			wlen := 0
			for _, bt := range bts {
				n, err := aa.FileHdl.Write(bt)
				if err != nil {
					return wlen, err
				}
				wlen += n
			}
			return wlen, nil
		}
	}
	fpath := ""
	for {
		fpath = fmt.Sprintf("%s%d.log", aa.fpkey, aa.Index)
		if fstat, err := os.Stat(fpath); err != nil && os.IsNotExist(err) {
			// 文件不存在， 创建文件所在的文件夹
			parent := filepath.Dir(fpath)
			if _, err := os.Stat(parent); os.IsNotExist(err) {
				os.MkdirAll(parent, 0644)
			}
			break
		} else if err == nil && fstat.Size() > aa.MaxSize {
			// 文件存在，但大小超过限制， 继续下一个索引
			aa.Index++
			continue
		} else if err == nil && fstat.IsDir() {
			// 跳过，文件名存在同名文件夹
			z.Logf("[_logfile]: check store file error -> %s, %s", fpath, " is dir")
			if aa.CloseFunc != nil {
				aa.CloseFunc(aa)
			}
			return 0, fmt.Errorf("file name exists as directory") //
		} else if err != nil {
			// 跳过，无法处理，遇到不可预知错误
			z.Logf("[_logfile]: check store file error -> %s, %s", fpath, err.Error())
			if aa.CloseFunc != nil {
				aa.CloseFunc(aa)
			}
			return 0, fmt.Errorf("unknown error: %s", err.Error()) //
		} else {
			// 文件存在，而且大小合适， 继续写入
			break
		}
	}
	var err error // 创建 + 追加 + 只写
	aa.FileHdl, err = os.OpenFile(fpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// 跳过，无法处理， 无法打开或者创建文件夹
		z.Logf("[_logfile]: open store file error -> %s, %s", fpath, err.Error())
		if aa.CloseFunc != nil {
			aa.CloseFunc(aa)
		}
		return 0, fmt.Errorf("open store file error: %s", err.Error())
	}
	defer aa._check()
	wlen := 0
	for _, bt := range bts {
		n, err := aa.FileHdl.Write(bt)
		if err != nil {
			return wlen, err
		}
		wlen += n
	}
	return wlen, nil
}

func (aa *RollingFile) _check() {
	aa.alive = time.Now().Unix() + 10
	if aa.timer != nil {
		return // 执行器存在， 跳过
	}
	// 创建回收器， 延迟关闭
	aa.timer = time.AfterFunc(time.Second*5, aa._close)
}

func (aa *RollingFile) _close() {
	if aa.alive > time.Now().Unix() {
		// 创建回收器，继续迭代检查
		aa.timer.Reset(time.Second * 5)
		return
	}
	if aa.CloseFunc != nil {
		aa.CloseFunc(aa)
	}
	aa.FileHdl.Close()
	aa.FileHdl = nil
	aa.timer = nil // 删除执行器
}

var _ io.Writer = (*RollingFile)(nil)

func (aa *RollingFile) Write(bts []byte) (int, error) {
	return aa.Writex(bts)
}

var _ io.Closer = (*RollingFile)(nil)

func (aa *RollingFile) Close() error {
	aa.flock.Lock()
	defer aa.flock.Unlock()
	if aa.CloseFunc != nil {
		aa.CloseFunc(aa)
	}
	if aa.timer != nil {
		aa.timer.Stop()
		aa.timer = nil
	}
	if aa.FileHdl != nil {
		aa.FileHdl.Close()
		aa.FileHdl = nil
	}
	return nil
}
