package kwdog2

// https://github.com/gojue/ecapture

// 使用 ecapture 进行流量捕获和分析

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	_ "unsafe"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/proc"
	"github.com/suisrc/zgg/z/ze/wss"
)

type KwbeeConfig struct {
	Disabled bool     `json:"disabled"`
	AddrPort string   `json:"addr"`
	Ecapture string   `json:"ecapture"`
	CmdArgs  []string `json:"cmdargs"`
	Ewstoken string   `json:"ewstoken"`
}

// 初始化方法， 处理 hdl 的而外配置接口 443
type InitKwbeeFunc func(hdl *KwbeeHandler, zgg *z.Zgg)

func InitKwbee(ifn InitKwbeeFunc) {

	flag.BoolVar(&C.Kwbee2.Disabled, "b2disabled", true, "是否禁用kwbee2")
	flag.StringVar(&C.Kwbee2.AddrPort, "b2addr", "127.0.0.1:28254", "代理服务器地址和端口")
	flag.StringVar(&C.Kwbee2.Ecapture, "b2ecapture", "ecapture", "ecapture命令")
	flag.Var(z.NewStrArr(&C.Kwbee2.CmdArgs, []string{}), "b2cmdargs", "ecapture参数")
	flag.StringVar(&C.Kwbee2.Ewstoken, "b2ewstoken", zc.GenStr("kwbee-", 38), "ewstoken")

	z.Register("14-kwbee2", func(zgg *z.Zgg) z.Closed {
		if C.Kwbee2.Disabled {
			z.Logn("[_kwbee2_]: disabled")
			return nil
		}
		hdl := &KwbeeHandler{
			addr:    C.Kwbee2.AddrPort,
			process: proc.NewProcess(nil, C.Kwbee2.Ecapture, C.Kwbee2.CmdArgs...),
			handler: wss.NewHandler(C.Kwbee2.Ewstoken, nil),
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
	handler *wss.Handler
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

func (hdl *KwbeeHandler) NewHook(key string, req *http.Request, sender wss.SendFunc, cancel func()) (string, wss.Hook, error) {
	return key, hdl, nil
}

func (hdl *KwbeeHandler) Close() error {
	return nil
}

func (hdl *KwbeeHandler) Receive(code byte, data []byte) (byte, []byte, error) {
	z.Logn("[_kwbee2_]: received data, code:", code, "data:", string(data))
	return 0, nil, nil
}
