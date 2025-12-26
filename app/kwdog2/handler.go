package kwdog2

import (
	"flag"
	"net/http"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/cfg"
	"github.com/suisrc/zgg/ze/gte"
	"github.com/suisrc/zgg/ze/gtw"
)

var (
	C = struct {
		Kwdog2 Kwdog2Config
	}{}
)

type Kwdog2Config struct {
	RootPath string            `json:"rootpath"` // api
	ServAddr string            `json:"servaddr"` // 默认 127.0.0.1:80
	AuthAddr string            `json:"authz"`    // ??
	Routers  map[string]string `json:"routers"`
	Sites    []string          `json:"sites"`
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *KwdogApi, srv z.IServer)

func Init() {
	Init3(nil)
}

func Init3(ifn InitializFunc) {
	cfg.Register(&C)

	flag.StringVar(&C.Kwdog2.RootPath, "k2rp", "", "kwdog root path")
	flag.StringVar(&C.Kwdog2.ServAddr, "k2sa", "http://127.0.0.1:80", "serve addr")
	flag.StringVar(&C.Kwdog2.AuthAddr, "k2aa", "", "authz serve addr")
	flag.Var(cfg.NewStrMap(&C.Kwdog2.Routers, z.HM{}), "k2rmap", "other api routing")
	flag.Var(cfg.NewStrArr(&C.Kwdog2.Sites, []string{}), "k2sites", "kwdog flag domain site")

	z.Register("01-kwdog2", func(srv z.IServer) z.Closed {
		var err error
		api := &KwdogApi{
			RootPath:   C.Kwdog2.RootPath,
			ServAddr:   C.Kwdog2.ServAddr,
			AuthAddr:   C.Kwdog2.AuthAddr,
			Routers:    C.Kwdog2.Routers,
			Sites:      C.Kwdog2.Sites,
			GatewayMap: make(map[string]http.Handler),
		}
		api.RecordPool = gte.NewRecordStdout()
		api.BufferPool = gtw.NewBufferPool(0, 0)
		api.GtwDefault, err = gtw.NewTargetGateway(api.ServAddr, api.BufferPool)
		if err != nil {
			z.Printf("register kwdow2 error, %v", err.Error())
			srv.ServeStop()
			return nil
		}
		api.GtwDefault.ProxyName = "default-gateway"
		api.GtwDefault.RecordPool = api.RecordPool
		api.GtwDefault.Authorizer = gte.NewAuthorize1(api.Sites, api.AuthAddr)

		srv.AddRouter(api.RootPath, api.ServeHTTP)

		if ifn != nil {
			ifn(api, srv) // 初始化方法
		}
		return nil
	})

}

type KwdogApi struct {
	RootPath string
	ServAddr string
	AuthAddr string
	Routers  map[string]string
	Sites    []string

	RecordPool gtw.RecordPool          // 记录池
	BufferPool gtw.BufferPool          // 缓存池
	GtwDefault *gtw.GatewayProxy       // 默认网关
	Authorizer gtw.Authorizer          // 默认记录
	GatewayMap map[string]http.Handler // 路由网关
	GatewayLck sync.RWMutex
}

func (aa *KwdogApi) GetProxy(kk string) http.Handler {
	aa.GatewayLck.RLock()
	defer aa.GatewayLck.RUnlock()
	return aa.GatewayMap[kk]
}

func (aa *KwdogApi) NewProxy(kk, vv string) (http.Handler, error) {
	aa.GatewayLck.Lock()
	defer aa.GatewayLck.Unlock()
	proxy, err := gtw.NewTargetGateway(vv, aa.BufferPool) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa.Authorizer == nil {
		aa.Authorizer = gte.NewAuthorize1(aa.Sites, "") // 只记录日志
	}

	proxy.ProxyName = kk + "-gateway"
	proxy.RecordPool = aa.RecordPool
	proxy.Authorizer = aa.Authorizer

	aa.GatewayMap[kk] = proxy
	return proxy, nil
}

// ServeHTTP
func (aa *KwdogApi) ServeHTTP(zrc *z.Ctx) bool {
	rw := zrc.Writer
	rr := zrc.Request
	for kk, vv := range aa.Routers {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if z.C.Debug {
			z.Printf("[_routing]: %s[%s] -> %s\n", kk, rr.URL.Path, vv)
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk, vv); err != nil {
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		} else {
			proxy.ServeHTTP(rw, rr) // next
		}
		return true
	}
	// --------------------------------------------------------------
	if z.C.Debug {
		z.Printf("[_routing]: %s -> %s\n", rr.URL.Path, aa.ServAddr)
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
	return true
}
