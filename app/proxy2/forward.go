package proxy2

import (
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/crt"
	"github.com/suisrc/zgg/ze/gte"
	"github.com/suisrc/zgg/ze/gtw"
)

var (
	C = struct {
		Proxy2 Proxy2Config
	}{}
)

type Proxy2Config struct {
	AddrPort string `json:"port" default:"0.0.0.0:12012"`
	AddrPtls string `json:"ptls" default:"0.0.0.0:12014"`
	Dual     bool   `json:"dual"`
	CrtCA    string `json:"cacrt"`
	KeyCA    string `json:"cakey"`
	IsSAA    bool   `json:"casaa"`
	Expiry   string `json:"expiry" default:"20y"`
	Syslog   string `json:"syslog"` // 日志发送地址
	Ttylog   bool   `json:"ttylog"` // 是否打印日志
}

// 不可使用， 考虑使用 eBPF 无侵入的方式
// https://github.com/cilium/cilium
// https://github.com/gojue/ecapture

// 初始化方法， 处理 api 的而外配置接口 12012
type InitializFunc func(api *Proxy2Api, zgg *z.Zgg)

func Init3(ifn InitializFunc) {
	zc.Register(&C)

	flag.StringVar(&(C.Proxy2.AddrPort), "p2port", "0.0.0.0:12012", "http server addr and port")
	flag.StringVar(&(C.Proxy2.AddrPtls), "p2ptls", "0.0.0.0:12014", "https server addr and port")
	flag.BoolVar(&(C.Proxy2.Dual), "p2dual", false, "是否同时启动 http 和 https")
	flag.StringVar(&(C.Proxy2.CrtCA), "p2crt", "", "CA证书文件")
	flag.StringVar(&(C.Proxy2.KeyCA), "p2key", "", "CA私钥文件")
	flag.BoolVar(&(C.Proxy2.IsSAA), "p2saa", false, "是否为中间证书")
	flag.StringVar(&(C.Proxy2.Expiry), "p2exp", "20y", "创建根证书的有效期")
	flag.StringVar(&C.Proxy2.Syslog, "k2syslog", "", "日志发送地址")
	flag.BoolVar(&C.Proxy2.Ttylog, "k2ttylog", false, "是否打印日志")

	z.Register("12-proxy2", func(zgg *z.Zgg) z.Closed {
		api := new(Proxy2Api)
		if err := api.Init(C.Proxy2); err != nil {
			zgg.ServeStop("register proxy2 error,", err.Error())
			return nil
		}
		if api.TLSConfig != nil { // https
			srv := &http.Server{Addr: C.Proxy2.AddrPtls, Handler: api, TLSConfig: api.TLSConfig}
			zgg.Servers = append(zgg.Servers, &z.Server{Key: "(PROXY-HTTPS)", Srv: srv})
		}
		if C.Proxy2.Dual || api.TLSConfig == nil { // http
			srv := &http.Server{Addr: C.Proxy2.AddrPort, Handler: api}
			zgg.Servers = append(zgg.Servers, &z.Server{Key: "(PROXY-HTTP1)", Srv: srv})
		}

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

func (api *Proxy2Api) Init(cfg Proxy2Config) error {
	abp := gtw.NewBufferPool(32*1024, 0)
	api.GtwDefault = &gtw.GatewayProxy{ReverseProxy: gtw.ReverseProxy{BufferPool: abp}}
	api.GtwDefault.Rewrite = func(r *gtw.ProxyRequest) {}
	api.GtwDefault.ProxyName = "proxy2-gateway"
	api.GtwDefault.RecordPool = gte.NewRecordSyslog(cfg.Syslog, "udp", 0, cfg.Ttylog)
	// api.GtwDefault.RecordPool = gte.NewRecordPrint()

	if cfg.CrtCA == "" || cfg.KeyCA == "" {
		return nil // 忽略 https
	}
	crtBts, err := os.ReadFile(cfg.CrtCA)
	if err != nil {
		if os.IsNotExist(err) {
			zc.Println("[_proxy2_]", "cacrt not found, build", cfg.CrtCA)
			// 创建根证书
			config := crt.CertConfig{"default": {
				Expiry: "20y",
				SubjectName: crt.SignSubject{
					Organization:     "default",
					OrganizationUnit: "default",
				},
			}}
			dir := filepath.Dir(cfg.CrtCA)
			if err := os.MkdirAll(dir, 0644); err != nil {
				return err
			}
			ca, err := crt.CreateCA(config, "default")
			if err != nil {
				return err
			}
			if err := os.WriteFile(cfg.CrtCA, []byte(ca.Crt), 0644); err != nil {
				return err
			}
			if err := os.WriteFile(cfg.KeyCA, []byte(ca.Key), 0644); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	keyBts, err := os.ReadFile(cfg.KeyCA)
	if err != nil {
		return err
	}
	config := crt.TLSAutoConfig{
		CaCrtBts: crtBts,
		CaKeyBts: keyBts,
		CertConf: crt.CertConfig{"default": {
			Expiry: "20y",
			SubjectName: crt.SignSubject{
				Organization:     "default",
				OrganizationUnit: "default",
			},
		}},
	}
	api.TLSConfig = &tls.Config{GetCertificate: (&config).GetCertificate}

	return nil
}

type Proxy2Api struct {
	TLSConfig  *tls.Config
	GtwDefault *gtw.GatewayProxy // 默认网关
}

// ServeHTTP
func (aa *Proxy2Api) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}

	if z.IsDebug() {
		zc.Printf("[_forward]: %s -> %s\n", rr.RemoteAddr, rr.URL.String())
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
