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

var (
	appname = strings.TrimSpace(string(appbyte))
	version = strings.TrimSpace(string(verbyte))
)

/**
 * 程序入口
 */
func main() {
	// z.Println = klog.Infoln
	// z.Printf = klog.Infof
	// z.Fatal = klog.Fatal
	// z.Fatalf = klog.Fatalf
	flag.StringVar(&app.C.Token, "token", "", "http server api token")
	z.Execute(appname, version, "(https://github.com/suisrc/k8skit) main")
}
