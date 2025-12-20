// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package z

import (
	"fmt"
	"os"
	"strings"
)

// 程序入口
func Execute() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("================ exit: panic,", err)
			os.Exit(1) // exit with panic
		}
	}()
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		CMDR["web"]() // run  def http server
		return        // wait for server stop
	}
	cmd := os.Args[1]
	if command, ok := CMDR[cmd]; ok {
		// 修改命令参数
		os.Args = append(os.Args[:1], os.Args[2:]...)
		command() // run command
		// flag.Parse() > flag.CommandLine.Parse(os.Args[2:])
	} else {
		fmt.Println("unknown command:", cmd)
	}
}

// ----------------------------------------------------------------------------

var (
	// Command Registry
	CMDR = map[string]func(){
		"web":     RunHttpServe,
		"version": PrintVersion,
	}
)

func RunHttpServe() {
	Printf("%s %s (https://github.com/suisrc/k8skit) starting...\n", Appname, Version)
	LoadConfig()
	// zgg server
	zgg := &Zgg{}
	if !zgg.ServeInit(zgg) {
		return // init error, exit
	}
	// run and wait http server
	zgg.RunAndWait(zgg.ServeHTTP)
}

func PrintVersion() {
	fmt.Printf("%s %s (https://github.com/suisrc/k8skit)\n", Appname, Version)
}

// func Exit(err error, code int) {
// 	fmt.Println("exit with error:", err.Error())
// 	os.Exit(code)
// }

// ----------------------------------------------------------------------------
// 重写服务

// func init() {
// 	CmdR["web"] = RunHttpServe
// }
// // 定制扩展 zgg 框架
// type Egg struct {
// 	Zgg
// 	//...
// }
// func (aa *Egg) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
// 	//...
// }
