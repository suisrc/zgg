// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 将所以得包补依赖引导到此文件中， 已便于清晰确认核心组件对外部的依赖
// 所有的依赖，包括项目中非闭包内容的所有依赖

package z

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/suisrc/zgg/z/zc"
)

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

	// Command Map Registry
	CMD = map[string]func(){
		"web":     RunHttpServe,
		"version": PrintVersion,
	}
)

// 注册默认方法
func Initializ() {
	// 注册配置函数
	zc.Register(C)

	flag.Var(zc.NewBoolVal(&(zc.C.Debug)), "debug", "debug mode")
	flag.Var(zc.NewBoolVal(&(zc.C.Print)), "print", "print mode")
	flag.Var(zc.NewStrVal(&(zc.C.Syslog)), "syslog", "logger to syslog server")
	flag.Var(zc.NewBoolVal(&(zc.C.LogTty)), "logtty", "logger to tty")
	flag.Var(zc.NewBoolVal(&(C.Server.Fxser)), "fxser", "http header flag xser-*")
	flag.Var(zc.NewBoolVal(&(C.Server.Local)), "local", "http server local mode")
	flag.StringVar(&(C.Server.Addr), "addr", "0.0.0.0", "http server addr")
	flag.IntVar(&(C.Server.Port), "port", 80, "http server Port")
	flag.IntVar(&(C.Server.Ptls), "ptls", 443, "https server Port")
	flag.BoolVar(&(C.Server.Dual), "dual", false, "running http and https server")
	flag.StringVar(&(C.Server.Engine), "eng", "map", "http server router engine")
	flag.StringVar(&(C.Server.ApiPath), "api", "", "http server api path")
	flag.StringVar(&(C.Server.TplPath), "tpl", "", "templates folder path")
	flag.StringVar(&(C.Server.ReqXrtd), "xrt", "", "X-Request-Rt default value")

	//  register default serve
	Register("90-server", RegisterDefaultHttpServe)
}

// ----------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------

// 请求数据
func ReadForm[T any](rr *http.Request, rb T) (T, error) {
	return zc.Map2ToStruct(rb, rr.URL.Query(), "form")
}

// 获取 traceID / 配置 traceID
func GetTraceID(request *http.Request) string {
	traceid := request.Header.Get("X-Request-Id")
	if traceid == "" {
		traceid = zc.GenStr("r", 32) // 创建请求ID, 用于追踪
		request.Header.Set("X-Request-Id", traceid)
	}
	return traceid
}
