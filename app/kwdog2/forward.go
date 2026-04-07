package kwdog2

// curl -k -x 127.0.0.1:12014 https://ipinfo.io

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

type ProxyConfig struct {
	Disabled bool   `json:"disabled"`
	AddrPort string `json:"port" default:"0.0.0.0:12012"`
	CrtCA    string `json:"cacrt"`
	KeyCA    string `json:"cakey"`
	IsSAA    bool   `json:"casaa"`
	Expiry   string `json:"expiry" default:"20y"`
	Syslog   string `json:"syslog"`  // 日志发送地址
	LogBody  bool   `json:"logBody"` // 是否打印请求体
	Record   int    `json:"record"`
}

// 不可使用， 考虑使用 eBPF 无侵入的方式
// https://github.com/cilium/cilium
// https://github.com/gojue/ecapture

// 初始化方法， 处理 api 的而外配置接口 12012
type InitProxyFunc func(api *Proxy2Api, zgg *z.Zgg)

func InitProxy(ifn InitProxyFunc) {

	flag.BoolVar(&C.Proxy2.Disabled, "p2disabled", true, "是否禁用proxy2")
	flag.StringVar(&C.Proxy2.AddrPort, "p2port", "0.0.0.0:12012", "代理服务器地址和端口")
	flag.StringVar(&C.Proxy2.CrtCA, "p2crt", "", "CA证书文件")
	flag.StringVar(&C.Proxy2.KeyCA, "p2key", "", "CA私钥文件")
	flag.BoolVar(&C.Proxy2.IsSAA, "p2saa", false, "是否为中间证书")
	flag.StringVar(&C.Proxy2.Expiry, "p2exp", "20y", "创建根证书的有效期")
	flag.StringVar(&C.Proxy2.Syslog, "p2syslog", "none", "日志发送地址, none: 表示不记录日志")
	flag.BoolVar(&C.Proxy2.LogBody, "p2logbody", false, "记录日志中的Body")
	flag.IntVar(&C.Proxy2.Record, "p2record", -1, "记录级别")

	z.Register("12-proxy2", func(zgg *z.Zgg) z.Closed {
		if C.Proxy2.Disabled {
			z.Logn("[_proxy2_]: disabled")
			return nil
		}

		// ...
		switch C.Proxy2.Record {
		case 0:
			RecordFrowardFunc = gte.ToRecord0
		case 1:
			RecordFrowardFunc = gte.ToRecord1
		}

		api := new(Proxy2Api)
		if err := api.Init(C.Proxy2); err != nil {
			zgg.ServeStop("register proxy2 error,", err.Error())
			return nil
		}
		zgg.Servers["(PROXY)"] = z.NewServer(api, C.Proxy2.AddrPort, nil)

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

func (api *Proxy2Api) Init(cfg ProxyConfig) error {
	api.GtwDefault = &gtw.ForwardProxy{}
	api.GtwDefault.BufferPool = gtw.NewBufferPool(32*1024, 0)
	api.GtwDefault.Rewrite = func(r *gtw.ProxyRequest) {}
	api.GtwDefault.ProxyName = "proxy2-gateway"
	if cfg.Syslog != "none" {
		api.GtwDefault.RecordPool = gte.NewRecordSyslog(
			cfg.Syslog,
			zc.C.Logger.Pty,
			zc.C.Logger.Tty,
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
	api.GtwDefault.TLSConfig = &tlsx.TLSAutoConfig{
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

type Proxy2Api struct {
	GtwDefault *gtw.ForwardProxy // 默认网关
}

// ServeHTTP
func (aa *Proxy2Api) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}
	if z.IsDebug() {
		z.Logf("[_proxy2_]: %s -> %s\n", rr.RemoteAddr, rr.URL.String())
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
