package kwdog2

// dog: 通常指家狗、野狗，也可作为动词表示"看守、监视", 它的存在就是管理 http 流量内容
// reverse, 反向代理服务， 主要用于请求网关， 也可以用于其他的反向代理场景
// curl -x 127.0.0.1:12006 ip.info
import (
	"flag"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/gte"
	"github.com/suisrc/zgg/z/ze/gtw"
)

// 反向代理服务配置规则
// value: def+ 前缀表示共享默认网关配置， 其他的表示独立网关配置
// key: @ 前缀表示多域名路由，格式为 @domain/path

type KwdogConfig struct {
	Disabled bool              `json:"disabled"`
	AddrPort string            `json:"addr" default:"0.0.0.0:12006"`
	NextAddr string            `json:"next"`    // 默认 127.0.0.1:80
	AuthAddr string            `json:"authz"`   // ??
	AuthSkip bool              `json:"askip"`   // 默认不跳过, 可以忽略鉴权
	Routers  map[string]string `json:"routers"` // 其他路由
	Rtrack   bool              `json:"rtrack"`  // 追踪路由
	Rauthz   string            `json:"rauthz"`  // 鉴权路由
	Sites    []string          `json:"sites"`   // 站点列表， 用于标记 _xc
	Logger   string            `json:"logger"`  // 日志发送地址
	LogBody  bool              `json:"logBody"` // 记录日志中的Body
	LogTty   bool              `json:"logTty"`  // 日志是否输出到终端
	Record   int               `json:"record"`
}

// 初始化方法， 处理 hdl 的而外配置接口
type InitKwdogFunc func(hdl *KwdogHandler, zgg *z.Zgg)

func InitKwdog(ifn InitKwdogFunc) {

	flag.BoolVar(&G.Kwdog2.Disabled, "k2disabled", false, "是否禁用kwdog2")
	flag.StringVar(&G.Kwdog2.AddrPort, "k2addr", "0.0.0.0:12006", "代理服务器地址和端口")
	flag.StringVar(&G.Kwdog2.NextAddr, "k2next", "http://127.0.0.1:80", "后端服务地址")
	flag.StringVar(&G.Kwdog2.AuthAddr, "k2auth", "", "认证服务地址， 默认只支持 f1kin 服务")
	flag.BoolVar(&G.Kwdog2.AuthSkip, "k2askip", false, "在存在鉴权头部信息时，是否跳过鉴权")
	flag.Var(z.NewStrMap(&G.Kwdog2.Routers, z.HM{}), "k2rmap", "其他服务转发")
	flag.BoolVar(&G.Kwdog2.Rtrack, "k2track", false, "是否记录其他路由的日志")
	flag.StringVar(&G.Kwdog2.Rauthz, "k2rauth", "", "其他路由是否进行鉴权")
	flag.Var(z.NewStrArr(&G.Kwdog2.Sites, []string{}), "k2sites", "需要标记 _xc 的站点")
	flag.StringVar(&G.Kwdog2.Logger, "k2logger", "", "日志发送地址， none: 表示不记录日志")
	flag.BoolVar(&G.Kwdog2.LogBody, "k2logbody", false, "记录日志中的Body")
	flag.IntVar(&G.Kwdog2.Record, "k2record", -1, "记录级别")

	z.Register("11-kwdog2", func(zgg *z.Zgg) z.Closed {
		if G.Kwdog2.Disabled {
			z.Logn("[_kwdog2_]: disabled")
			return nil
		}
		if strings.HasSuffix(G.Kwdog2.AddrPort, ":80") && G.Kwdog2.NextAddr == "http://127.0.0.1:80" {
			G.Kwdog2.NextAddr = "http://127.0.0.1:81" // 避免循环
			z.Logn("[_kwdog2_]: default next address changed to", G.Kwdog2.NextAddr)
		}

		// ...
		switch G.Kwdog2.Record {
		case 0:
			RecordReverseFunc = gte.ToRecord0
		case 1:
			RecordReverseFunc = gte.ToRecord1
		}

		hdl := new(KwdogHandler)
		if err := hdl.Init(G.Kwdog2); err != nil {
			zgg.ServeStop("register kwdog2 error,", err.Error())
			return nil
		}
		z.Logn("[_kwdog2_]: routers", hdl.RouterKey, "domains", hdl.DomainMap)
		zgg.Servers.Add(z.NewServer("(KWDOG)", hdl, G.Kwdog2.AddrPort, nil))

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})

}

func (hdl *KwdogHandler) Init(cfg KwdogConfig) error {
	var err error = nil
	var rsp gtw.RecordPool = nil
	if cfg.Logger != "none" {
		rsp = gte.NewRecorder(
			cfg.Logger,
			zc.G.Logger.Pty,
			cfg.LogTty,
			cfg.LogBody,
			RecordReverseFunc,
		)
	}
	hdl.GtwDefault, err = gtw.NewTargetGatewayV2(cfg.NextAddr)
	if err != nil {
		return err
	}
	hdl.GtwDefault.ProxyName = "kwdog2-gateway"
	hdl.GtwDefault.RecordPool = rsp
	hdl.GtwDefault.Authorizer = AuthzDefaultFunc(
		cfg.Sites,
		cfg.AuthAddr,
		cfg.AuthSkip,
	)
	if cfg.Rtrack {
		hdl.RecordPool = rsp
	}
	switch cfg.Rauthz {
	case "", "none", "no", "disable":
		// ignore
	case "default":
		hdl.Authorizer = hdl.GtwDefault.Authorizer
	case "logger":
		hdl.Authorizer = gte.NewAuthLogger(cfg.Sites)
	default:
		if strings.HasPrefix(cfg.Rauthz, "https://") ||
			strings.HasPrefix(cfg.Rauthz, "http://") {
			hdl.Authorizer = AuthzDefaultFunc(
				cfg.Sites,
				cfg.Rauthz,
				cfg.AuthSkip,
			)
		}
	}
	// 特殊的多域名路由情况， 以 @ 开头， 格式为 @domain/path, 其中 path 可省略， 默认根路径
	hdl.Routers = make(map[string]string)
	hdl.DomainMap = make(map[string][]string)
	// 解析所有路由
	for kk, vv := range cfg.Routers {
		hdl.Routers[kk] = vv
		if len(kk) > 2 && kk[0] == '@' {
			// 新增多域名路由
			host, path := kk[1:], ""
			if idx := strings.Index(host, "/"); idx > 0 {
				path = host[idx:]
				host = host[:idx]
			}
			// 加入路由队列中
			routers, exist := hdl.DomainMap[host]
			if !exist {
				routers = []string{}
			}
			routers = append(routers, path)
			hdl.DomainMap[host] = routers
			continue
		}
		// 默认路由
		hdl.RouterKey = append(hdl.RouterKey, kk)
	}
	// hdl.RoutersKey 按字符串长度倒序
	if len(hdl.RouterKey) > 1 {
		slices.SortFunc(hdl.RouterKey, func(l string, r string) int { return len(r) - len(l) })
	}
	// hdl.DomainMap 中的路径也按字符串长度倒序
	for host, paths := range hdl.DomainMap {
		if len(paths) > 1 {
			slices.SortFunc(paths, func(l string, r string) int { return len(r) - len(l) })
			hdl.DomainMap[host] = paths
		}
	}
	return nil
}

type KwdogHandler struct {
	Routers  map[string]string // 路由配置
	NextAddr string            // 后端服务地址
	// ----------------------------------------
	RecordPool gtw.RecordPool          // 记录池
	GtwDefault *gtw.GatewayProxy       // 默认网关
	Authorizer gtw.Authorizer          // 默认记录
	RouterMap  map[string]gtw.IGateway // 路由网关
	RouterKey  []string                // 目录网关
	DomainMap  map[string][]string     // 域名网关
	_svc_lock  sync.RWMutex
}

func (aa *KwdogHandler) GetProxy(kk string) gtw.IGateway {
	if aa.RouterMap == nil {
		return nil
	}
	aa._svc_lock.RLock()
	defer aa._svc_lock.RUnlock()
	return aa.RouterMap[kk]
}

func (aa *KwdogHandler) NewProxy(kk string) (gtw.IGateway, error) {
	aa._svc_lock.Lock()
	defer aa._svc_lock.Unlock()
	vv, ok := aa.Routers[kk]
	if !ok {
		return nil, fmt.Errorf("router not found: %s", kk)
	}
	share := false
	if strings.HasPrefix(vv, "def+") {
		// 需要网关处理
		share = true
		vv = vv[4:]
	}
	var gw *gtw.GatewayProxy
	var err error
	if strings.HasPrefix(vv, "domain+") {
		gw, err = gtw.NewCustomGatewayV2(vv[7:], "", nil)
	} else if strings.HasPrefix(vv, "domain-") {
		gw, err = gtw.NewCustomGatewayV2(vv[7:], "", gtw.TransportSkip)
	} else {
		gw, err = gtw.NewTargetGatewayV2(vv) // 创建目标URL
	}
	if err != nil {
		return nil, err
	}
	if aa.RouterMap == nil {
		aa.RouterMap = make(map[string]gtw.IGateway)
	}
	gw.ProxyName = strings.ReplaceAll(kk, "/", "_") + "-gateway"
	if share {
		// 共享默认网关配置
		gw.RecordPool = aa.GtwDefault.RecordPool
		gw.Authorizer = aa.GtwDefault.Authorizer
	} else {
		// 记录其他网关日志
		gw.RecordPool = aa.RecordPool
		gw.Authorizer = aa.Authorizer
	}
	aa.RouterMap[kk] = gw
	return gw, nil
}

func (aa *KwdogHandler) ProxyHTTP(rw http.ResponseWriter, rr *http.Request, kk string) {
	if proxy := aa.GetProxy(kk); proxy != nil {
		// 使用缓存的网关
		if z.IsDebug() {
			z.Logf("[_kwdog2_]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, aa.Routers[kk])
		}
		proxy.ServeHTTP(rw, rr) // next
	} else if proxy, err := aa.NewProxy(kk); err == nil {
		// 创建新的网关
		if z.IsDebug() {
			z.Logf("[_kwdog2_]: [%s] %s -> %s\n", proxy.GetProxyName(), rr.URL.Path, aa.Routers[kk])
		}
		proxy.ServeHTTP(rw, rr) // next
	} else {
		// 没有网关可用， 返回 502 错误
		if z.IsDebug() {
			z.Logf("[_kwdog2_]: [%s] %s -> %s, %v\n", kk, rr.URL.Path, aa.Routers[kk], err)
		}
		http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
	}
}

// ServeHTTP
func (aa *KwdogHandler) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.URL.Path == "/healthz" {
		z.JSON0(rr, rw, &z.Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
		return
	}
	// 代理路由服务
	if paths, exist := aa.DomainMap[rr.Host]; exist {
		for _, path := range paths {
			if !z.HasPathPrefix(rr.URL.Path, path) {
				continue // 数量少， 可以这么处理
			}
			kk := "@" + rr.Host + path
			aa.ProxyHTTP(rw, rr, kk)
			return
		}
	}
	// 代理路由服务
	for _, kk := range aa.RouterKey {
		if !z.HasPathPrefix(rr.URL.Path, kk) {
			continue // 数量少， 可以这么处理
		}
		aa.ProxyHTTP(rw, rr, kk)
		return
	}
	// --------------------------------------------------------------
	if z.IsDebug() {
		z.Logf("[_kwdog2_]: [%s] %s -> %s\n", aa.GtwDefault.ProxyName, rr.URL.Path, aa.NextAddr)
	}
	aa.GtwDefault.ServeHTTP(rw, rr)
}
