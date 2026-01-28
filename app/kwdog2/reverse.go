package kwdog2

import (
	"flag"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gte"
	"github.com/suisrc/zgg/z/ze/gtw"
)

var (
	C = struct {
		Kwdog2 Config
	}{}

	RecordFunc = gte.ToRecord0
)

type Config struct {
	Disabled bool              `json:"disabled"`
	AddrPort string            `json:"addr" default:"0.0.0.0:12006"`
	NextAddr string            `json:"next"`    // 默认 127.0.0.1:80
	AuthAddr string            `json:"authz"`   // ??
	AuthSkip bool              `json:"askip"`   // 默认不跳过, 可以忽略鉴权
	Routers  map[string]string `json:"routers"` // 其他路由
	Rtrack   bool              `json:"rtrack"`  // 追踪路由
	Sites    []string          `json:"sites"`   // 站点列表， 用于标记 _xc
	Syslog   string            `json:"syslog"`  // 日志发送地址
	LogNet   string            `json:"logudp"`  // 日志发送协议
	LogPri   int               `json:"logpri"`  // 日志优先级
	LogTty   bool              `json:"logtty"`  // 是否打印日志
	LogBody  bool              `json:"logBody"` // 记录日志中的Body
	Record   int               `json:"record"`
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *KwdogApi, zgg *z.Zgg)

func Init3(ifn InitializFunc) {
	z.Config(&C)

	flag.BoolVar(&C.Kwdog2.Disabled, "k2disabled", false, "是否禁用kwdog2")
	flag.StringVar(&C.Kwdog2.AddrPort, "k2addr", "0.0.0.0:12006", "监控认证端口")
	flag.StringVar(&C.Kwdog2.NextAddr, "k2next", "http://127.0.0.1:80", "后端服务地址")
	flag.StringVar(&C.Kwdog2.AuthAddr, "k2auth", "", "认证服务地址， 默认只支持 f1kin 服务")
	flag.BoolVar(&C.Kwdog2.AuthSkip, "k2askip", false, "在存在鉴权头部信息时，是否跳过鉴权")
	flag.Var(z.NewStrMap(&C.Kwdog2.Routers, z.HM{}), "k2rmap", "其他服务转发")
	flag.BoolVar(&C.Kwdog2.Rtrack, "k2rlog", false, "是否记录其他路由的日志")
	flag.Var(z.NewStrArr(&C.Kwdog2.Sites, []string{}), "k2sites", "需要标记 _xc 的站点")
	flag.StringVar(&C.Kwdog2.Syslog, "k2syslog", "", "日志发送地址， none: 表示不记录日志")
	flag.StringVar(&C.Kwdog2.LogNet, "k2lognet", "udp", "日志发送协议")
	flag.IntVar(&C.Kwdog2.LogPri, "k2logpri", 0, "日志优先级")
	flag.BoolVar(&C.Kwdog2.LogTty, "k2logtty", false, "是否打印日志")
	flag.BoolVar(&C.Kwdog2.LogBody, "k2logbody", false, "记录日志中的Body")
	flag.IntVar(&C.Kwdog2.Record, "k2record", 0, "记录级别")

	z.Register("11-kwdog2", func(zgg *z.Zgg) z.Closed {
		if C.Kwdog2.Disabled {
			z.Println("[_kwdog2_]: disabled")
			return nil
		}

		// ...
		switch C.Kwdog2.Record {
		case 1:
			RecordFunc = gte.ToRecord1
		}

		var err error
		api := &KwdogApi{Config: C.Kwdog2}

		if api.Config.Syslog != "none" {
			api.RecordPool = gte.NewRecordSyslog(
				api.Config.Syslog,
				api.Config.LogNet,
				api.Config.LogPri,
				api.Config.LogTty,
				api.Config.LogBody,
				RecordFunc,
			)
		}
		api.BufferPool = gtw.NewBufferPool(32*1024, 0)
		api.GtwDefault, err = gtw.NewTargetGateway(
			api.Config.NextAddr,
			api.BufferPool,
		)
		if err != nil {
			zgg.ServeStop("register kwdow2 error,", err.Error())
			return nil
		}
		api.GtwDefault.ProxyName = "kwdog2-gateway"
		api.GtwDefault.RecordPool = api.RecordPool
		api.GtwDefault.Authorizer = gte.NewAuthorize1(
			api.Config.Sites,
			api.Config.AuthAddr,
			api.Config.AuthSkip,
		)
		// api.Authorizer = gte.NewLoggerOnly(api.Sites)
		for kk := range api.Config.Routers {
			api.RouterKey = append(api.RouterKey, kk)
		}
		// api.RoutersKey 按字符串长度倒序
		slices.SortFunc(api.RouterKey, func(l string, r string) int { return -len(l) + len(r) })
		z.Println("[_kwdog2_]: routers", api.RouterKey)

		zgg.Servers["(KWDOG)"] = &http.Server{Addr: api.Config.AddrPort, Handler: api}

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

type KwdogApi struct {
	Config Config
	// ----------------------------------------
	RecordPool gtw.RecordPool          // 记录池
	BufferPool gtw.BufferPool          // 缓存池
	GtwDefault *gtw.GatewayProxy       // 默认网关
	Authorizer gtw.Authorizer          // 默认记录
	RouterSvc  map[string]gtw.IGateway // 路由网关
	RouterKey  []string
	_svc_lock  sync.RWMutex
}

func (aa *KwdogApi) GetProxy(kk string) gtw.IGateway {
	if aa.RouterSvc == nil {
		return nil
	}
	aa._svc_lock.RLock()
	defer aa._svc_lock.RUnlock()
	return aa.RouterSvc[kk]
}

func (aa *KwdogApi) NewProxy(kk string) (gtw.IGateway, error) {
	aa._svc_lock.Lock()
	defer aa._svc_lock.Unlock()
	vv := aa.Config.Routers[kk]
	gw, err := gtw.NewTargetGateway(vv, aa.BufferPool) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa.RouterSvc == nil {
		aa.RouterSvc = make(map[string]gtw.IGateway)
	}
	gw.ProxyName = strings.ReplaceAll(kk, "/", "_") + "-gateway"
	if aa.Config.Rtrack {
		gw.RecordPool = aa.RecordPool
		gw.Authorizer = aa.Authorizer
	}
	aa.RouterSvc[kk] = gw
	return gw, nil
}

// ServeHTTP
func (aa *KwdogApi) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}

	for _, kk := range aa.RouterKey {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			if z.IsDebug() {
				z.Printf("[_kwdog2_]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, aa.Config.Routers[kk])
			}
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk); err == nil {
			if z.IsDebug() {
				z.Printf("[_kwdog2_]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, aa.Config.Routers[kk])
			}
			proxy.ServeHTTP(rw, rr) // next
		} else {
			if z.IsDebug() {
				z.Printf("[_kwdog2_]: [%s] %s -> %s, %v\n", kk, rr.URL.Path, aa.Config.Routers[kk], err)
			}
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	// --------------------------------------------------------------
	if z.IsDebug() {
		z.Printf("[_kwdog2_]: [%s] %s -> %s\n", aa.GtwDefault.ProxyName, rr.URL.Path, aa.Config.NextAddr)
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
