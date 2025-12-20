package main

import (
	_ "embed"
	"strings"

	_ "github.com/suisrc/zgg/app"
	_ "github.com/suisrc/zgg/cmd"
	"github.com/suisrc/zgg/z"
	_ "github.com/suisrc/zgg/ze/rdx"
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
	z.Execute(appname, version, "(https://github.com/suisrc/k8skit) main")
}
