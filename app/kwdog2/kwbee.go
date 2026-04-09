package kwdog2

// https://github.com/gojue/ecapture

// 使用 ecapture 进行流量捕获和分析

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	_ "unsafe"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/proc"
	"github.com/suisrc/zgg/z/ze/wsz"
)

// FixCmdArgs 替换命令行参数中的占位符，例如 <pid> 替换为当前进程的 PID
var FixCmdArgs = func(args []string) []string {
	for i, arg := range args {
		switch arg {
		case "<pid>":
			args[i] = fmt.Sprintf("%d", os.Getpid())
		case "<ws-addr>":
			args[i] = "ws://" + C.Kwbee2.AddrPort + "/ecapture"
		}
	}
	return args
}

type KwbeeConfig struct {
	Disabled bool     `json:"disabled"`
	AddrPort string   `json:"addr"`
	Command  string   `json:"command"`
	CmdArgs  []string `json:"cmdargs"`
	TtyLog   string   `json:"ttylog"`
}

// 初始化方法， 处理 hdl 的而外配置接口 443
type InitKwbeeFunc func(hdl *KwbeeHandler, zgg *z.Zgg)

func InitKwbee(ifn InitKwbeeFunc) {

	// 只捕获出口流量，且长度小于32768字节，排除本地回环地址的流量
	PCAP := `"outbound and len < 32768 and not dst net 127.0.0.0/8"`

	flag.BoolVar(&C.Kwbee2.Disabled, "b2disabled", true, "是否禁用kwbee2")
	flag.StringVar(&C.Kwbee2.AddrPort, "b2addr", "127.0.0.1:28255", "代理服务器地址和端口")
	flag.StringVar(&C.Kwbee2.Command, "b2command", "ecapture", "ecapture命令")
	flag.Var(z.NewStrArr(&C.Kwbee2.CmdArgs, []string{"tls", "--pid", "<pid>", "--eventaddr", "<ws-addr>", PCAP}), "b2cmdargs", "ecapture参数")
	flag.StringVar(&C.Kwbee2.TtyLog, "b2ttylog", "", "ecapture: 适配并截取日志, stdout: 直接输出到控制台")

	z.Register("14-kwbee2", func(zgg *z.Zgg) z.Closed {
		if C.Kwbee2.Disabled {
			z.Logn("[_kwbee2_]: disabled")
			return nil
		}
		var plog io.Writer
		switch C.Kwbee2.TtyLog {
		case "discard":
			plog = io.Discard
		case "stdout":
			plog = os.Stdout
		case "ecapture":
			plog = EcaptureLogger{}
		default:
			plog = io.Discard
			C.Kwbee2.CmdArgs = append(C.Kwbee2.CmdArgs, "--logaddr", "ws://"+C.Kwbee2.AddrPort+"/logging")
		}
		hdl := &KwbeeHandler{
			addr:    C.Kwbee2.AddrPort,
			process: proc.NewProcess(plog, C.Kwbee2.Command, FixCmdArgs(C.Kwbee2.CmdArgs)...),
			handler: wsz.NewHandler(nil),
		}
		hdl.handler.NewHook = hdl.NewHook
		zgg.Servers.Add(hdl)

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})
}

var _ z.Server = (*KwbeeHandler)(nil)

type KwbeeHandler struct {
	addr    string
	process proc.Process
	handler *wsz.Handler
	server  *http.Server
}

func (hdl *KwbeeHandler) Name() string {
	return "(KWBEE)"
}

func (hdl *KwbeeHandler) Addr() string {
	return hdl.addr
}

func (hdl *KwbeeHandler) RunServe() {
	hdl.server = &http.Server{Addr: hdl.addr, Handler: hdl.handler}
	go func() {
		if err := hdl.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			z.Exit(fmt.Sprintf("[_kwbee2_]: server exit error: %s\n", err))
		}
	}()
	if z.IsDebug() {
		z.Logn("[_kwbee2_]: starting process:", hdl.process.String())
	}
	time.Sleep(1 * time.Second)
	hdl.process.Serve()
}

func (hdl *KwbeeHandler) Shutdown(ctx context.Context) error {
	if hdl.process != nil {
		_ = hdl.process.Stop(0) // 发送 SIGTERM 信号，等待进程退出, 忽略错误
	}
	if hdl.server != nil {
		if err := hdl.server.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (hdl *KwbeeHandler) NewHook(key string, req *http.Request, sender wsz.SendFunc, cancel func()) (string, wsz.Hook, error) {
	return key, hdl, nil
}

func (hdl *KwbeeHandler) Close() error {
	return nil
}

func (hdl *KwbeeHandler) Receive(code byte, data []byte) (byte, []byte, error) {
	z.Logn("[_kwbee2_]: received data, code:", code, "data:", string(data))
	return 0, nil, nil
}

// ----------------------------------------------------------------------------------------------------

type EcaptureLogger struct{}

func (EcaptureLogger) Write(p []byte) (int, error) {
	// [90m2026-04-09T08:49:20+08:00[0m [32mINF[0m PID:0,
	buf := bytes.NewBuffer(nil)
	// key := []byte(" [32mINF[0m ") // [34:48]
	key := []byte(" [32m") // [34:40]
	// filter PID: info
	pid := []byte("PID:")
	dec := []byte("Decoding ")
	for i, line := range bytes.Split(p, []byte{'\n'}) {
		if len(line) > 48 && bytes.Equal(line[34:40], key) {
			if buf.Len() > 0 {
				z.Logn("[ecapture]:", buf.String())
				buf.Reset() // 清空缓冲区
			}
			if bts := line[48:]; !bytes.HasPrefix(bts, pid) && !bytes.HasPrefix(bts, dec) {
				buf.Write(bts)
			}
		} else if buf.Len() > 0 {
			buf.WriteByte(' ')
			buf.Write(line)
		} else if i == 0 {
			buf.Write(line)
		}
	}
	if buf.Len() > 0 {
		z.Logn("[ecapture]:", buf.String())
		buf.Reset() // 清空缓冲区
	}
	// z.Logn("[ecapture]:", string(p))
	return len(p), nil
}
