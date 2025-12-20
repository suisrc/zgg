package main

import (
	_ "embed"
	"flag"
	"strings"

	"github.com/suisrc/zgg/app"
	_ "github.com/suisrc/zgg/cmd"
	"github.com/suisrc/zgg/z"
	_ "github.com/suisrc/zgg/ze/rdx"
	// "k8s.io/klog/v2"
)

//go:embed vname
var appbyte []byte

//go:embed version
var verbyte []byte

// //go:embed www/* www/**/*
// var wwwFS embed.FS

var (
	appname = strings.TrimSpace(string(appbyte))
	version = strings.TrimSpace(string(verbyte))
)

/**
 * 程序入口
 */
func main() {
	// front2.Init(wwwFS) // 由于需要 wwwFS参数，必须人工初始化
	flag.StringVar(&app.C.Token, "token", "", "http server api token")
	z.Execute(appname, version, "(https://github.com/suisrc/k8skit) main")
}
