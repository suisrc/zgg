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
	return zc.C.Debug
}

var (
	Printf = func(format string, v ...any) {
		zc.Log.Output(2, func(b []byte) []byte { return fmt.Appendf(b, format, v...) })
	}

	Println = func(v ...any) {
		zc.Log.Output(2, func(b []byte) []byte { return fmt.Appendln(b, v...) })
	}

	Fatalf = func(format string, v ...any) {
		zc.Log.Output(2, func(b []byte) []byte { return fmt.Appendf(b, format, v...) })
		os.Exit(1)
	}

	Fatalln = func(v ...any) {
		zc.Log.Output(2, func(b []byte) []byte { return fmt.Appendln(b, v...) })
		os.Exit(1)
	}

	Printl3 = func(v ...any) {
		zc.Log.Output(3, func(b []byte) []byte { return fmt.Appendln(b, v...) })
	}

	Config    = zc.Register
	ToStr     = zc.ToStr
	ToStr2    = zc.ToStr2
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

	// Command Map Registry
	CMD = map[string]func(){
		"web":     RunHttpServe,
		"version": PrintVersion,
	}
)

// 注册默认方法
func Initializ() {
	// 注册配置函数
	Config(C)

	flag.Var(NewBoolVal(&(zc.C.Debug)), "debug", "debug mode")
	flag.Var(NewBoolVal(&(zc.C.Print)), "print", "print mode")
	flag.Var(NewStrVal(&(zc.C.Syslog), ""), "syslog", "logger to syslog server")
	flag.Var(NewBoolVal(&(zc.C.LogTty)), "logtty", "logger to tty")
	flag.Var(NewBoolVal(&(C.Server.Fxser)), "fxser", "http header flag xser-*")
	flag.Var(NewBoolVal(&(C.Server.Local)), "local", "http server local mode")
	flag.StringVar(&(C.Server.Addr), "addr", "0.0.0.0", "http server addr")
	flag.IntVar(&(C.Server.Port), "port", 80, "http server Port")
	flag.IntVar(&(C.Server.Ptls), "ptls", 443, "https server Port")
	flag.BoolVar(&(C.Server.Dual), "dual", false, "running http and https server")
	flag.StringVar(&(C.Server.Engine), "eng", "map", "http server router engine")
	flag.StringVar(&(C.Server.ApiPath), "api", "", "http server api path")
	flag.StringVar(&(C.Server.TplPath), "tpl", "", "templates folder path")
	flag.StringVar(&(C.Server.ReqXrtd), "xrt", "", "X-Request-Rt default value")

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
