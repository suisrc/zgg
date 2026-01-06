package main

import (
	_ "embed"
	"strings"

	"github.com/suisrc/zgg/z"
	_ "github.com/suisrc/zgg/ze/rdx"

	// _ "github.com/suisrc/zgg/ze/tls_file"
	// _ "github.com/suisrc/zgg/ze/tls_auto"

	"github.com/suisrc/zgg/app/kwdog2"
	"github.com/suisrc/zgg/app/kwlog2"
	// _ "github.com/suisrc/zgg/app/zhe"
	_ "github.com/suisrc/zgg/cmd"
)

//go:embed vname
var app_ []byte

//go:embed version
var ver_ []byte

// //go:embed www/* www/**/*
// var www_ embed.FS

var (
	_app = strings.TrimSpace(string(app_))
	_ver = strings.TrimSpace(string(ver_))
)

/**
 * 程序入口
 */
func main() {
	// zc.CFG_ENV = "KIT"
	// z.HttpServeDef = false // 标记是否启动默认 HTTP 服务， z.RegisterDefaultHttpServe

	// front2.Init3(www_, nil) // 前端应用，由于需要 wwwFS参数，必须人工初始化
	kwdog2.Init3(nil) // API边车网关， 通过 Sidecar 模式保护主服务
	kwlog2.Init3(nil) // 采集器日志, 为 fluentbit agent 提供 HTTP 收集日志功能

	z.Execute(_app, _ver, "(https://github.com/suisrc/zgg)")
}
