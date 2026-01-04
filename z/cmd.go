// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package z

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/suisrc/zgg/z/zc"
)

// 程序入口
func Execute(appname, version, appinfo string) {
	AppName, Version, AppInfo = appname, version, appinfo
	// 发生错误需要截取到异常， 所以这里忽略 defer 方法
	// defer func() {
	// 	if err := recover(); err != nil {
	// 		fmt.Println("================ exit: panic,", err)
	// 		os.Exit(1) // exit with panic
	// 	}
	// }()
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
	PrintVersion()
	// config
	var cfs string
	flag.StringVar(&cfs, "c", "", "config file path")
	flag.Parse() // command line arguments
	zc.LoadConfig(cfs)
	// server
	zgg := &Zgg{}
	if zgg.ServeInit() {
		zgg.RunServe(nil, "")
	}
}

func PrintVersion() {
	println(AppName, Version, AppInfo)
}
