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
	Routers  map[string]string `json:"routers"`  // 其他路由
	Routerl  bool              `json:"routerl"`  // 是否记录日志
	Sites    []string          `json:"sites"`    // 站点列表， 用于标记 _xc
	Syslog   string            `json:"syslog"`   // 日志发送地址
	Ttylog   bool              `json:"ttylog"`   // 是否打印日志
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *KwdogApi, srv z.IServer)

func Init() {
	Init3(nil)
}

func Init3(ifn InitializFunc) {
	cfg.Register(&C)

	flag.StringVar(&C.Kwdog2.RootPath, "k2rp", "", "根路径， 默认监控所有接口")
	flag.StringVar(&C.Kwdog2.ServAddr, "k2sa", "http://127.0.0.1:80", "后端服务地址")
	flag.StringVar(&C.Kwdog2.AuthAddr, "k2aa", "", "认证服务地址， 默认只支持 f1kin 服务")
	flag.Var(cfg.NewStrMap(&C.Kwdog2.Routers, z.HM{}), "k2rmap", "其他服务转发")
	flag.BoolVar(&C.Kwdog2.Routerl, "k2rlog", false, "认证服务地址， 默认只支持 f1kin 服务")
	flag.Var(cfg.NewStrArr(&C.Kwdog2.Sites, []string{}), "k2sites", "需要标记 _xc 的站点")
	flag.StringVar(&C.Kwdog2.Syslog, "k2syslog", "", "日志发送地址")
	flag.BoolVar(&C.Kwdog2.Ttylog, "k2ttylog", false, "是否打印日志")

	z.Register("01-kwdog2", func(srv z.IServer) z.Closed {
		var err error
		api := &KwdogApi{
			RootPath:   C.Kwdog2.RootPath,
			ServAddr:   C.Kwdog2.ServAddr,
			AuthAddr:   C.Kwdog2.AuthAddr,
			Routers:    C.Kwdog2.Routers,
			Routerl:    C.Kwdog2.Routerl,
			Sites:      C.Kwdog2.Sites,
			GatewayMap: make(map[string]gtw.IGateway),
		}
		api.RecordPool = gte.NewRecordSyslog(C.Kwdog2.Syslog, "udp", 0, C.Kwdog2.Ttylog)
		api.BufferPool = gtw.NewBufferPool(32*1024, 0)
		api.GtwDefault, err = gtw.NewTargetGateway(api.ServAddr, api.BufferPool)
		if err != nil {
			z.Printf("register kwdow2 error, %v", err.Error())
			srv.ServeStop()
			return nil
		}
		api.GtwDefault.ProxyName = "default-gateway"
		api.GtwDefault.RecordPool = api.RecordPool
		api.GtwDefault.Authorizer = gte.NewAuthorize1(api.Sites, api.AuthAddr)

		// api.Authorizer = gte.NewAuthorize1(api.Sites, "") // 同 NewLoggerOnly
		// // api.Authorizer = gte.NewLoggerOnly(api.Sites)

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
	Routerl  bool
	Sites    []string

	RecordPool gtw.RecordPool          // 记录池
	BufferPool gtw.BufferPool          // 缓存池
	GtwDefault *gtw.GatewayProxy       // 默认网关
	Authorizer gtw.Authorizer          // 默认记录
	GatewayMap map[string]gtw.IGateway // 路由网关
	GatewayLck sync.RWMutex
}

func (aa *KwdogApi) GetProxy(kk string) gtw.IGateway {
	aa.GatewayLck.RLock()
	defer aa.GatewayLck.RUnlock()
	return aa.GatewayMap[kk]
}

func (aa *KwdogApi) NewProxy(kk, vv string) (gtw.IGateway, error) {
	aa.GatewayLck.Lock()
	defer aa.GatewayLck.Unlock()
	proxy, err := gtw.NewTargetGateway(vv, aa.BufferPool) // 创建目标URL
	if err != nil {
		return nil, err
	}
	proxy.ProxyName = strings.ReplaceAll(kk, "/", "_") + "-gateway"
	if aa.Routerl {
		proxy.RecordPool = aa.RecordPool
		proxy.Authorizer = aa.Authorizer
	}

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
		if proxy := aa.GetProxy(kk); proxy != nil {
			if z.C.Debug {
				z.Printf("[_routing]: [%s] %s -> %s\n", proxy.GetProxyName(), zrc.Action, vv)
			}
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk, vv); err != nil {
			if z.C.Debug {
				z.Printf("[_routing]: [%s] %s -> %s, %v\n", kk, zrc.Action, vv, err)
			}
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		} else {
			if z.C.Debug {
				z.Printf("[_routing]: [%s] %s -> %s\n", proxy.GetProxyName(), zrc.Action, vv)
			}
			proxy.ServeHTTP(rw, rr) // next
		}
		return true
	}
	// --------------------------------------------------------------
	if z.C.Debug {
		z.Printf("[_routing]: [%s] %s -> %s\n", aa.GtwDefault.ProxyName, zrc.Action, aa.ServAddr)
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
	return true
}
