package main

import (
	_ "embed"
	"os"
	"strings"

	"github.com/suisrc/zgg/app/front2"
	"github.com/suisrc/zgg/app/kwdog2"
	_ "github.com/suisrc/zgg/cmd"
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	_ "github.com/suisrc/zgg/z/ze/log"
	_ "github.com/suisrc/zgg/z/ze/rdx"
	// _ "github.com/suisrc/zgg/app/zhe" // 测试模块
)

//go:embed vname
var app_ []byte

//go:embed version
var ver_ []byte

// //go:embed www/* www/**/*
// var www_ embed.FS

func main() {
	_app := strings.TrimSpace(string(app_))
	_ver := strings.TrimSpace(string(ver_))
	// 启动日志追踪， 显示打印日志的位置， 与 build -w 不可同时使用， 默认关闭
	zc.CFG_ENV, zc.C.Logger.File = "KIT", false
	// zc.C.Logger.Syslog, zc.C.Logger.Tty = "udp://klog.default.svc:5141", true
	// z.HttpServeDef = false // 标记是否启动默认 HTTP 服务， z.RegisterDefaultHttpServe

	front2.Init3(os.DirFS("www"), nil) // 前端应用, 使用系统文件夹中文件
	kwdog2.Init3(nil)                  // API(反向/正向)网关， 通过 Sidecar 模式保护内部服务
	// kwlog2.Init3(nil)      // 采集器日志, 为 fluentbit agent 提供 HTTP 收集日志功能

	z.Execute(_app, _ver, "(https://github.com/suisrc/zgg.git)")
}
