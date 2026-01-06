package kwdog2

import (
	"flag"
	"net/http"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gte"
	"github.com/suisrc/zgg/ze/gtw"
)

var (
	C = struct {
		Proxy2 Proxy2Config
	}{}
)

type Proxy2Config struct {
	Port   int    `json:"port" default:"12012"`
	Ptls   int    `json:"ptls" default:"12014"`
	CrtCA  string `json:"cacrt"`
	KeyCA  string `json:"cakey"`
	IsSAA  bool   `json:"casaa"`
	Syslog string `json:"syslog"` // 日志发送地址
	Ttylog bool   `json:"ttylog"` // 是否打印日志
}

// 不可使用， 考虑使用 eBPF 无侵入的方式
// https://github.com/cilium/cilium
// https://github.com/gojue/ecapture

// 初始化方法， 处理 api 的而外配置接口 12012
type InitializFunc func(api *Proxy2Api, zgg *z.Zgg)

func Init3(ifn InitializFunc) {
	zc.Register(&C)

	flag.IntVar(&(C.Proxy2.Port), "p2port", 80, "http server Port")
	flag.IntVar(&(C.Proxy2.Ptls), "p2ptls", 443, "https server Port")
	flag.StringVar(&(C.Proxy2.CrtCA), "p2crt", "", "CA证书文件")
	flag.StringVar(&(C.Proxy2.KeyCA), "p2key", "", "CA私钥文件")
	flag.BoolVar(&(C.Proxy2.IsSAA), "p2saa", false, "是否为中间证书")
	flag.StringVar(&C.Proxy2.Syslog, "k2syslog", "", "日志发送地址")
	flag.BoolVar(&C.Proxy2.Ttylog, "k2ttylog", false, "是否打印日志")

	z.Register("12-proxy2", func(zgg *z.Zgg) z.Closed {
		api := new(Proxy2Api)
		api.Init(C.Proxy2)

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

func (api *Proxy2Api) Init(cfg Proxy2Config) {
	abp := gtw.NewBufferPool(32*1024, 0)
	api.GtwDefault = &gtw.GatewayProxy{ReverseProxy: gtw.ReverseProxy{BufferPool: abp}}
	api.GtwDefault.Rewrite = func(r *gtw.ProxyRequest) {}
	api.GtwDefault.ProxyName = "proxy2-gateway"
	api.GtwDefault.RecordPool = gte.NewRecordSyslog(cfg.Syslog, "udp", 0, cfg.Ttylog)
	// api.GtwDefault.RecordPool = gte.NewRecordPrint()
}

type Proxy2Api struct {
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
