package kwdog2

import (
	"flag"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gte"
	"github.com/suisrc/zgg/ze/gtw"
)

var (
	C = struct {
		Kwdog2 Kwdog2Config
	}{}

	RecordFunc = gte.ToRecord0
)

type Kwdog2Config struct {
	AddrPort string            `json:"addr" default:"0.0.0.0:12006"`
	NextAddr string            `json:"next"`    // 默认 127.0.0.1:80
	AuthAddr string            `json:"authz"`   // ??
	AuthSkip bool              `json:"askip"`   // 默认不跳过, 可以忽略鉴权
	Routers  map[string]string `json:"routers"` // 其他路由
	Rtrack   bool              `json:"rtrack"`  // 追踪路由
	Sites    []string          `json:"sites"`   // 站点列表， 用于标记 _xc
	Syslog   string            `json:"syslog"`  // 日志发送地址
	Ttylog   bool              `json:"ttylog"`  // 是否打印日志
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *KwdogApi, zgg *z.Zgg)

func Init3(ifn InitializFunc) {
	zc.Register(&C)

	flag.StringVar(&C.Kwdog2.AddrPort, "k2addr", "0.0.0.0:12006", "监控认证端口")
	flag.StringVar(&C.Kwdog2.NextAddr, "k2next", "http://127.0.0.1:80", "后端服务地址")
	flag.StringVar(&C.Kwdog2.AuthAddr, "k2auth", "", "认证服务地址， 默认只支持 f1kin 服务")
	flag.BoolVar(&C.Kwdog2.AuthSkip, "k2askip", false, "在存在鉴权头部信息时，是否跳过鉴权")
	flag.Var(zc.NewStrMap(&C.Kwdog2.Routers, z.HM{}), "k2rmap", "其他服务转发")
	flag.BoolVar(&C.Kwdog2.Rtrack, "k2rlog", false, "是否记录其他路由的日志")
	flag.Var(zc.NewStrArr(&C.Kwdog2.Sites, []string{}), "k2sites", "需要标记 _xc 的站点")
	flag.StringVar(&C.Kwdog2.Syslog, "k2syslog", "", "日志发送地址")
	flag.BoolVar(&C.Kwdog2.Ttylog, "k2ttylog", false, "是否打印日志")

	z.Register("11-kwdog2", func(zgg *z.Zgg) z.Closed {
		var err error
		api := &KwdogApi{Config: C.Kwdog2}
		api.RecordPool = gte.NewRecordSyslog(api.Config.Syslog, "udp", 0, api.Config.Ttylog, RecordFunc)
		api.BufferPool = gtw.NewBufferPool(32*1024, 0)
		api.GtwDefault, err = gtw.NewTargetGateway(api.Config.NextAddr, api.BufferPool)
		if err != nil {
			zgg.ServeStop("register kwdow2 error,", err.Error())
			return nil
		}
		api.GtwDefault.ProxyName = "kwdog2-gateway"
		api.GtwDefault.RecordPool = api.RecordPool
		api.GtwDefault.Authorizer = gte.NewAuthorize1(api.Config.Sites, api.Config.AuthAddr, api.Config.AuthSkip)
		// api.Authorizer = gte.NewLoggerOnly(api.Sites)

		zgg.Servers["(KWDOG)"] = &http.Server{Addr: api.Config.AddrPort, Handler: api}

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

type KwdogApi struct {
	Config Kwdog2Config
	// ----------------------------------------
	RecordPool gtw.RecordPool          // 记录池
	BufferPool gtw.BufferPool          // 缓存池
	GtwDefault *gtw.GatewayProxy       // 默认网关
	Authorizer gtw.Authorizer          // 默认记录
	_end_rmap  map[string]gtw.IGateway // 路由网关
	_end_lock  sync.RWMutex
}

func (aa *KwdogApi) GetProxy(kk string) gtw.IGateway {
	if aa._end_rmap == nil {
		return nil
	}
	aa._end_lock.RLock()
	defer aa._end_lock.RUnlock()
	return aa._end_rmap[kk]
}

func (aa *KwdogApi) NewProxy(kk, vv string) (gtw.IGateway, error) {
	aa._end_lock.Lock()
	defer aa._end_lock.Unlock()
	gw, err := gtw.NewTargetGateway(vv, aa.BufferPool) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa._end_rmap == nil {
		aa._end_rmap = make(map[string]gtw.IGateway)
	}
	gw.ProxyName = strings.ReplaceAll(kk, "/", "_") + "-gateway"
	if aa.Config.Rtrack {
		gw.RecordPool = aa.RecordPool
		gw.Authorizer = aa.Authorizer
	}
	aa._end_rmap[kk] = gw
	return gw, nil
}

// ServeHTTP
func (aa *KwdogApi) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}

	for kk, vv := range aa.Config.Routers {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			if z.IsDebug() {
				zc.Printf("[_reverse]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, vv)
			}
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk, vv); err == nil {
			if z.IsDebug() {
				zc.Printf("[_reverse]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, vv)
			}
			proxy.ServeHTTP(rw, rr) // next
		} else {
			if z.IsDebug() {
				zc.Printf("[_reverse]: [%s] %s -> %s, %v\n", kk, rr.URL.Path, vv, err)
			}
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
	}
	// --------------------------------------------------------------
	if z.IsDebug() {
		zc.Printf("[_reverse]: [%s] %s -> %s\n", aa.GtwDefault.ProxyName, rr.URL.Path, aa.Config.NextAddr)
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
