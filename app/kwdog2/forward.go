package kwdog2

import (
	"net/http"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/ze/gte"
	"github.com/suisrc/zgg/ze/gtw"
)

// 不可使用， 考虑使用 eBPF 无侵入的方式
// https://github.com/cilium/cilium
// https://github.com/gojue/ecapture
func Init2() {

	z.Register("01-kwdog2", func(srv z.IServer) z.Closed {
		api := &ForwardApi{}
		arp := gte.NewRecordPrint()
		abp := gtw.NewBufferPool(32*1024, 0)
		api.GtwDefault = &gtw.GatewayProxy{ReverseProxy: gtw.ReverseProxy{BufferPool: abp}}
		api.GtwDefault.Director = func(r *http.Request) {}
		api.GtwDefault.ProxyName = "forward-gateway"
		api.GtwDefault.RecordPool = arp

		srv.AddRouter("", api.ServeHTTP)
		return nil
	})

}

type ForwardApi struct {
	GtwDefault *gtw.GatewayProxy // 默认网关
}

// ServeHTTP
func (aa *ForwardApi) ServeHTTP(zrc *z.Ctx) bool {
	rw := zrc.Writer
	rr := zrc.Request
	if z.IsDebug() {
		z.Printf("[_routing]: %s -> %s\n", rr.RemoteAddr, rr.URL.String())
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
	return true
}
