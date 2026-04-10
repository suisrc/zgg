package kwdog2

// https://github.com/gojue/ecapture

// 使用 ecapture 进行流量捕获和分析

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	_ "unsafe"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
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
	Disabled  bool     `json:"disabled"`
	AddrPort  string   `json:"addr"`
	Command   string   `json:"command"`
	CmdArgs   []string `json:"cmdargs"`
	Logger    string   `json:"logger"`
	Webdocket int      `json:"webdocket"`
}

// 初始化方法， 处理 hdl 的而外配置接口 443
type InitKwbeeFunc func(hdl *KwbeeHandler, zgg *z.Zgg)

func InitKwbee(ifn InitKwbeeFunc) {

	// 只捕获出口流量，且长度小于32768字节，排除本地回环地址的流量
	PCAP := `"outbound and len < 32768 and not dst net 127.0.0.0/8"`

	flag.BoolVar(&C.Kwbee2.Disabled, "b2disabled", true, "是否禁用kwbee2")
	flag.StringVar(&C.Kwbee2.AddrPort, "b2addr", "127.0.0.1:28255", "代理服务器地址和端口")
	flag.StringVar(&C.Kwbee2.Command, "b2command", "monitor", "monitor 命令")
	flag.Var(z.NewStrArr(&C.Kwbee2.CmdArgs, []string{"tls", "--pid", "<pid>", "--logaddr", "<ws-addr>", PCAP}), "b2cmdargs", "ecapture参数")
	flag.StringVar(&C.Kwbee2.Logger, "b2logger", "", "ecapture: 适配并截取日志, stdout: 直接输出到控制台, 空字符串: 不输出(可以通过 logaddr 输出日志)")
	flag.IntVar(&C.Kwbee2.Webdocket, "b2webdocket", 1, "启用 websocket 方式")

	z.Register("14-kwbee2", func(zgg *z.Zgg) z.Closed {
		if C.Kwbee2.Disabled {
			z.Logn("[_kwbee2_]: disabled")
			return nil
		}
		var plog io.Writer
		switch C.Kwbee2.Logger {
		case "stdout":
			plog = os.Stdout
		case "ecapture":
			plog = ecaptureLogger{}
		default:
			plog = io.Discard
		}
		if C.Kwbee2.Webdocket > 1 {
			C.Kwbee2.Webdocket = 1
		}
		// 优先使用 command 中的参数， 如果参数不存在，在使用 args 中的参数
		cmd, args := proc.ParseCmd(C.Kwbee2.Command)
		if len(args) == 0 {
			args = C.Kwbee2.CmdArgs
		}
		hdl := &KwbeeHandler{
			address: C.Kwbee2.AddrPort,
			process: proc.NewProcess(plog, cmd, FixCmdArgs(args)...),
		}
		hdl.handler = wsz.NewHandler(hdl.NewHook, C.Kwbee2.Webdocket)
		zgg.Servers.Add(hdl)

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})
}

var _ z.Server = (*KwbeeHandler)(nil)

type KwbeeHandler struct {
	address string
	process proc.Process
	handler http.Handler
	server  *http.Server
}

func (hdl *KwbeeHandler) Name() string {
	return "(KWBEE)"
}

func (hdl *KwbeeHandler) Addr() string {
	if hdl.process == nil {
		return hdl.address
	}
	msg := hdl.address + " | " + hdl.process.String()
	if len(msg) > 64 {
		return msg[:64] + "..."
	}
	return msg
}

func (hdl *KwbeeHandler) RunServe() {
	hdl.server = &http.Server{Addr: hdl.address, Handler: hdl.handler}
	go func() {
		if err := hdl.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			z.Exit(fmt.Sprintf("[_kwbee2_]: server exit error: %s\n", err))
		}
	}()
	if z.IsDebug() {
		z.Logn("[_kwbee2_]:", hdl.process.String())
	}
	time.Sleep(1 * time.Second)
	if err := hdl.process.Serve(); err != nil {
		z.Exit(fmt.Sprintf("[_kwbee2_]: process exit error: %s\n", err))
	}
}

func (hdl *KwbeeHandler) Shutdown(ctx context.Context) error {
	if hdl.process != nil {
		_ = hdl.process.Stop(0) // 发送 SIGTERM 信号，等待进程退出, 忽略错误
	}
	if hdl.server != nil {
		_ = hdl.server.Shutdown(ctx) // 关闭服务器，等待当前请求处理完成, 忽略错误
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
	if bts, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		data = bts // 解码成功，使用解码后的数据
	}
	rmap := map[string]any{}
	if err := json.Unmarshal(data, &rmap); err != nil {
		z.Logn("[ecapture]: [not json]", string(data))
		return 0, nil, nil
	}
	delete(rmap, "level")
	delete(rmap, "time")
	z.Logn("[ecapture]:", zc.ToStrText(rmap, "Description", "message"))
	return 0, nil, nil
}

// ----------------------------------------------------------------------------------------------------
// ----------------------------------------------------------------------------------------------------
// ----------------------------------------------------------------------------------------------------

type ecaptureLogger struct{}

func (ecaptureLogger) Write(p []byte) (int, error) {
	// [90m2026-04-09T08:49:20+08:00[0m [32mINF[0m PID:0,
	buf := bytes.NewBuffer(nil)
	key := []byte(" [32m") // [34:40]
	// key := []byte(" [32mINF[0m ") // [34:48]
	// pid := []byte("PID:0,")
	// dec := []byte("Decoding ")
	for i, line := range bytes.Split(p, []byte{'\n'}) {
		if len(line) > 48 && bytes.Equal(line[34:40], key) {
			if buf.Len() > 0 {
				z.Logn("[ecapture]:", buf.String())
				buf.Reset() // 清空缓冲区
			}
			// if bts := line[48:]; !bytes.HasPrefix(bts, pid) && !bytes.HasPrefix(bts, dec) {
			// 	buf.Write(bts)
			// }
			buf.Write(line[48:])
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
