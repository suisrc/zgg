// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package z

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/suisrc/zgg/z/zc"
)

// 程序入口
func Execute(appname, version, appinfo string) {
	AppName, Version, AppInfo = appname, version, appinfo
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		CMD["web"]() // run  def http server
		return       // wait for server stop
	}
	cmd := os.Args[1]
	if command, ok := CMD[cmd]; ok {
		// 修改命令参数
		os.Args = append(os.Args[:1], os.Args[2:]...)
		command() // run command
		// flag.Parse() > flag.CommandLine.Parse(os.Args[2:])
	} else {
		fmt.Println("unknown command:", cmd)
	}
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// zgc, zgg 中不存在第三方依赖，所有依赖都在此文件中 cmd 包中 引入
// 包括项目中非闭包内容的所有依赖， 便于清晰确认核心组件对外部的依赖

func IsDebug() bool {
	return zc.G.Debug
}

var (
	// Command Map Registry
	CMD = map[string]func(){
		"web":     RunHttpServe,
		"version": PrintVersion,
	}

	// 日志函数， 也可以直接使用 slog 包，这个包含文件和行号的追踪功能
	Logf = zc.Logf
	Logn = zc.Logn
	Logz = zc.Logz
	Exit = zc.Exit
	// Deprecated: 已于v0.5.1中废弃 保留只是为了兼容旧版本，实际调用 Logn
	Println = zc.Logn
	// Deprecated: 已于v0.5.1中废弃 保留只是为了兼容旧版本，实际调用 Logf
	Printf = zc.Logf

	// 其他工具函数
	Config    = zc.Register
	ToStr     = zc.ToStr
	HexStr    = hex.EncodeToString
	GenStr    = zc.GenStr
	GenUUIDv4 = zc.GenUUIDv4
	UnicodeTo = zc.UnicodeToRunes

	GetHostname  = zc.GetHostname
	GetNamespace = zc.GetNamespace
	GetLocAreaIp = zc.GetLocAreaIp
	GetServeName = zc.GetServeName
	GetFuncInfo  = zc.GetFuncInfo

	NewBoolVal = zc.NewBoolVal
	NewIntVal  = zc.NewIntVal
	NewStrVal  = zc.NewStrVal
	NewStrArr  = zc.NewStrArr
	NewStrMap  = zc.NewStrMap
)

// 注册默认方法
func Initializ() {
	// 注册配置函数
	Config(G)

	flag.Var(NewBoolVal(&(zc.G.Debug)), "debug", "debug mode")
	flag.Var(NewBoolVal(&(zc.G.Print)), "print", "print mode")
	flag.Var(NewBoolVal(&(G.Server.Fxser)), "fxser", "http header flag xser-*")
	flag.Var(NewBoolVal(&(G.Server.Local)), "local", "http server local mode")
	flag.StringVar(&(G.Server.Addr), "addr", "0.0.0.0", "http server addr")
	flag.IntVar(&(G.Server.Port), "port", 80, "http server Port")
	flag.IntVar(&(G.Server.Ptls), "ptls", 443, "https server Port")
	flag.BoolVar(&(G.Server.Dual), "dual", false, "running http and https server")
	flag.StringVar(&(G.Server.Engine), "eng", "map", "http server router engine")
	flag.StringVar(&(G.Server.ApiRoot), "api", "", "http server api root")
	flag.StringVar(&(G.Server.TplPath), "tpl", "", "templates folder path")
	flag.StringVar(&(G.Server.ReqXrtd), "xrt", "", "X-Request-Rt default value")

	//  register default serve
	Register("90-server", RegisterHttpServe)
}

func RunHttpServe() {
	PrintVersion()
	Initializ()
	// parse command line arguments
	var cfs string
	flag.StringVar(&cfs, "c", "", "config file path")
	flag.Parse()
	// parse config file
	zc.LoadConfig(cfs)
	// running server
	zgg := &Zgg{}
	if zgg.ServeInit() {
		zgg.RunServe()
	}
}

// -----------------------------------------------------------------------------------------------------

// 请求数据
func ReadForm[T any](rr *http.Request, rb T) (T, error) {
	return zc.Map2ToStruct(rb, rr.URL.Query(), "form")
}

// 响应数据
func WriteRespBytes(rw http.ResponseWriter, ctype string, code int, data []byte) {
	h := rw.Header()
	// See https://go.dev/issue/66343.
	h.Del("Content-Length")
	// 设置 X-Content-Type-Options: nosniff 后，浏览器会严格遵循服务器返回的 Content-Type，不会尝试猜测资源类型。
	h.Set("X-Content-Type-Options", "nosniff")
	// 响应数据
	h.Set("Content-Type", ctype)
	rw.WriteHeader(code)
	rw.Write(data)
}

// 前缀匹配
func HasPathPrefix(path string, pre string) bool {
	if elen := len(pre); elen == 0 {
		return true // 对比 pre 为空，直接返回 true
	} else if alen := len(path); alen == 0 {
		return false // 参考 path 为空，直接返回 false
	} else if alen < elen || path[:elen] != pre {
		return false // pre 不是 path 前缀，返回 false
	} else {
		return alen == elen || pre[elen-1] == '/' || path[elen] == '/'
	}
	// 检索一次完成，之前的方法(如下)需要检索2次字符串(== 和 HasPrefix)
	// if len(pre) == 0 || path == pre {
	// 	return true
	// }
	// if pre[len(pre)-1] == '/' {
	// 	return strings.HasPrefix(path, pre)
	// }
	// return strings.HasPrefix(path, pre+"/")
}

type Slice[T any] []T

// 追加一条数据到数据集中
func (s *Slice[T]) Add(vals ...T) {
	*s = append(*s, vals...)
}

// 删除满足条件的第一条数据
func (s *Slice[T]) Del(fn func(val T) bool) {
	if idx := slices.IndexFunc(*s, fn); idx >= 0 {
		*s = slices.Delete(*s, idx, idx+1)
	}
}

// 校验数据是否存在，存在替换，不存在追加
func (s *Slice[T]) Set(fn func(val T) bool, val T) {
	if idx := slices.IndexFunc(*s, fn); idx >= 0 {
		(*s)[idx] = val
	} else {
		*s = append(*s, val)
	}
}

// 获取满足条件的第一条数据
func (s *Slice[T]) Get(fn func(val T) bool) (val T, ok bool) {
	if idx := slices.IndexFunc(*s, fn); idx >= 0 {
		return (*s)[idx], true
	}
	var zero T
	return zero, false
}
