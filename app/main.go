package main

import (
	_ "embed"
	"strings"

	_ "github.com/suisrc/zgg/cmd"
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	_ "github.com/suisrc/zgg/z/ze/log/syslog"
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
	zc.CFG_ENV, zc.C.LogTff = "KIT", false
	// zc.C.Syslog, zc.C.LogTty = "udp://klog.default.svc:5141", true
	// z.HttpServeDef = false // 标记是否启动默认 HTTP 服务， z.RegisterDefaultHttpServe
	// zc.LogTrackFile = true // 启动日志追踪， 显示打印日志的位置， 与 build -w 不可同时使用

	// front2.Init3(os.DirFS("www"), nil) // 前端应用, 使用系统文件夹中文件
	// kwlog2.Init3(nil) // 采集器日志, 为 fluentbit agent 提供 HTTP 收集日志功能
	// kwdog2.Init3(nil, nil) // API(反向/正向)网关， 通过 Sidecar 模式保护内部服务

	z.Execute(_app, _ver, "(https://github.com/suisrc/zgg.git)")
}
