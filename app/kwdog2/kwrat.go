package kwdog2

// rat: 通常指体型较大的家鼠、褐家鼠，也可作为动词表示"背叛、告密", 它的存在就是监控 http 流量内容
// forward proxy 代理服务器， 监听 12012 端口， 接收客户端的 http 请求， 转发到目标服务器，并将响应返回给客户端
// curl -k -x 127.0.0.1:12012 https://ipinfo.io

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/gte"
	"github.com/suisrc/zgg/z/ze/gtw"
	"github.com/suisrc/zgg/z/ze/tlsx"
)

type KwratConfig struct {
	Disabled bool   `json:"disabled"`
	AddrPort string `json:"addr"`
	CrtCA    string `json:"cacrt"`
	KeyCA    string `json:"cakey"`
	IsSAA    bool   `json:"casaa"`
	Expiry   string `json:"expiry" default:"20y"`
	Logger   string `json:"logger"`  // 日志发送地址
	LogBody  bool   `json:"logBody"` // 是否打印请求体
	LogTty   bool   `json:"logTty"`  // 日志是否输出到终端
	Record   int    `json:"record"`
}

// 不可使用， 考虑使用 eBPF 无侵入的方式
// https://github.com/cilium/cilium
// https://github.com/gojue/ecapture

// 初始化方法， 处理 hdl 的而外配置接口 12012
type InitKwratFunc func(hdl *KwratHandler, zgg *z.Zgg)

func InitKwrat(ifn InitKwratFunc) {

	flag.BoolVar(&C.Kwrat2.Disabled, "p2disabled", true, "是否禁用proxy2")
	flag.StringVar(&C.Kwrat2.AddrPort, "p2addr", "0.0.0.0:12012", "代理服务器地址和端口")
	flag.StringVar(&C.Kwrat2.CrtCA, "p2crt", "", "CA证书文件")
	flag.StringVar(&C.Kwrat2.KeyCA, "p2key", "", "CA私钥文件")
	flag.BoolVar(&C.Kwrat2.IsSAA, "p2saa", false, "是否为中间证书")
	flag.StringVar(&C.Kwrat2.Expiry, "p2exp", "20y", "创建根证书的有效期")
	flag.StringVar(&C.Kwrat2.Logger, "p2logger", "none", "日志发送地址, none: 表示不记录日志")
	flag.BoolVar(&C.Kwrat2.LogBody, "p2logbody", false, "记录日志中的Body")
	flag.IntVar(&C.Kwrat2.Record, "p2record", -1, "记录级别")

	z.Register("12-kwrat2", func(zgg *z.Zgg) z.Closed {
		if C.Kwrat2.Disabled {
			z.Logn("[_kwrat2_]: disabled")
			return nil
		}

		// ...
		switch C.Kwrat2.Record {
		case 0:
			RecordFrowardFunc = gte.ToRecord0
		case 1:
			RecordFrowardFunc = gte.ToRecord1
		}

		hdl := new(KwratHandler)
		if err := hdl.Init(C.Kwrat2); err != nil {
			zgg.ServeStop("register kwrat2 error,", err.Error())
			return nil
		}
		zgg.Servers.Add(z.NewServer("(KWRAT)", hdl, C.Kwrat2.AddrPort, nil))

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})

}

func (hdl *KwratHandler) Init(cfg KwratConfig) error {
	hdl.GtwDefault = &gtw.ForwardProxy{}
	hdl.GtwDefault.BufferPool = gtw.NewBufferPool(32*1024, 0)
	hdl.GtwDefault.Rewrite = func(r *gtw.ProxyRequest) {}
	hdl.GtwDefault.ProxyName = "kwrat2-gateway"
	if cfg.Logger != "none" {
		hdl.GtwDefault.RecordPool = gte.NewRecorder(
			cfg.Logger,
			zc.C.Logger.Pty,
			cfg.LogTty,
			cfg.LogBody,
			RecordFrowardFunc,
		)
	}
	if cfg.CrtCA == "" || cfg.KeyCA == "" {
		return nil // 忽略 https
	}
	crtBts, err := os.ReadFile(cfg.CrtCA)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// 创建根证书
		z.Logn("[_proxy2_]", "crtca not found, build", cfg.CrtCA)
		config := tlsx.CertConfig{"default": {
			Expiry: "20y",
			SubjectName: tlsx.SignSubject{
				Organization:     "default",
				OrganizationUnit: "default",
			},
		}}
		dir := filepath.Dir(cfg.CrtCA)
		if err := os.MkdirAll(dir, 0644); err != nil {
			return err
		}
		ca, err := tlsx.CreateCA(config, "default")
		if err != nil {
			return err
		}
		if err := os.WriteFile(cfg.CrtCA, []byte(ca.Crt), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(cfg.KeyCA, []byte(ca.Key), 0644); err != nil {
			return err
		}
	}
	keyBts, err := os.ReadFile(cfg.KeyCA)
	if err != nil {
		return err
	}
	hdl.GtwDefault.TLSConfig = &tlsx.TLSAutoConfig{
		CaCrtBts: crtBts,
		CaKeyBts: keyBts,
		IsSaCert: cfg.IsSAA,
		CertConf: tlsx.CertConfig{"default": {
			Expiry: "20y",
			SubjectName: tlsx.SignSubject{
				Organization:     "default",
				OrganizationUnit: "default",
			},
		}},
	}

	return nil
}

type KwratHandler struct {
	GtwDefault *gtw.ForwardProxy // 默认网关
}

// ServeHTTP
func (aa *KwratHandler) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}
	if z.IsDebug() {
		z.Logf("[_kwrat2_]: %s -> %s\n", rr.RemoteAddr, rr.URL.String())
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
