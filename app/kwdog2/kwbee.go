package kwdog2

// https://github.com/gojue/ecapture

// 使用 ecapture 进行流量捕获和分析

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "unsafe"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/proc"
)

// FixCmdArgs 替换命令行参数中的占位符，例如 <pid> 替换为当前进程的 PID
var FixCmdArgs = func(args []string) []string {
	for i, arg := range args {
		switch arg {
		case "<pid>":
			args[i] = fmt.Sprintf("%d", os.Getpid())
		}
	}
	return args
}

type KwbeeConfig struct {
	Disabled bool     `json:"disabled"`
	Command  string   `json:"command"`
	CmdArgs  []string `json:"cmdargs"`
}

// 初始化方法， 处理 hdl 的而外配置接口 443
type InitKwbeeFunc func(hdl *KwbeeHandler, zgg *z.Zgg)

func InitKwbee(ifn InitKwbeeFunc) {

	flag.BoolVar(&C.Kwbee2.Disabled, "b2disabled", true, "是否禁用kwbee2")
	flag.StringVar(&C.Kwbee2.Command, "b2command", "monitor", "monitor 命令")
	flag.Var(z.NewStrArr(&C.Kwbee2.CmdArgs, []string{"-cpid", "<pid>"}), "b2cmdargs", "monitor 参数")

	z.Register("14-kwbee2", func(zgg *z.Zgg) z.Closed {
		if C.Kwbee2.Disabled {
			z.Logn("[_kwbee2_]: disabled")
			return nil
		}
		// 优先使用 command 中的参数， 如果参数不存在，在使用 args 中的参数
		cmd, args := proc.ParseCmd(C.Kwbee2.Command)
		if len(args) == 0 {
			args = C.Kwbee2.CmdArgs
		}
		hdl := &KwbeeHandler{}
		hdl.process = proc.NewProcess(hdl, cmd, FixCmdArgs(args)...)
		zgg.Servers.Add(hdl)

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})
}

var _ z.Server = (*KwbeeHandler)(nil)

type KwbeeHandler struct {
	process proc.Process
}

func (hdl *KwbeeHandler) Name() string {
	return "(KWBEE)"
}

func (hdl *KwbeeHandler) Addr() string {
	str := hdl.process.String()
	if len(str) < 36 {
		return str
	}
	return str[:36] + "..."
}

func (hdl *KwbeeHandler) RunServe() {
	if err := hdl.process.Serve(); err != nil {
		z.Exit(fmt.Sprintf("[_kwbee2_]: process exit error: %s\n", err))
	}
}

func (hdl *KwbeeHandler) Shutdown(ctx context.Context) error {
	_ = hdl.process.Stop(0) // 发送 SIGTERM 信号，等待进程退出, 忽略错误
	return nil
}

func (hdl *KwbeeHandler) Write(p []byte) (n int, err error) {
	z.Logn("[_kwbee2_]:", string(p))
	return len(p), nil
}
