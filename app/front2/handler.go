package front2

import (
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gtw"
)

var (
	C = struct {
		Front2 Config
	}{}
)

type Config struct {
	ShowPath   string            `json:"f2show"`  // 显示 www 文件夹资源
	IsNative   bool              `json:"native"`  // 使用原生文件服务
	Index      string            `json:"index"`   // 默认首页文件名, index.html
	Indexs     map[string]string `json:"indexs"`  // index map, 多索引系统，不能已 / 结尾
	Routers    map[string]string `json:"routers"` // 路由表
	TmplRoot   string            `json:"tproot"`  // 根目录, /ROOT_PATH, 构建时可以在运行时替换，用于静态资源路径替换
	TmplSuffix []string          `json:"suffix"`  // 替换文件后缀, .html .htm .css .map .js
	TmplPrefix []string          `json:"prefix"`  // 替换文件前缀, app. umi. runtime.
	ChangeFile bool              `json:"change"`  // 支持文件变动
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *IndexApi, zgg *z.Zgg)

func Init3(www fs.FS, ifn InitializFunc) {
	z.Config(&C)

	flag.StringVar(&C.Front2.ShowPath, "f2show", "", "show www resource uri")
	flag.BoolVar(&C.Front2.IsNative, "f2native", false, "use native file server")
	flag.StringVar(&C.Front2.Index, "f2index", "index.html", "index file name")
	flag.Var(z.NewStrMap(&C.Front2.Indexs, z.HM{"/zgg": "index.htm"}), "f2indexs", "index file map")
	flag.Var(z.NewStrMap(&C.Front2.Routers, z.HM{}), "f2routers", "router path replace")
	flag.StringVar(&C.Front2.TmplRoot, "f2trpath", "/ROOT_PATH", "root path, empty is disabled")
	flag.Var(z.NewStrArr(&C.Front2.TmplSuffix, []string{".html", ".htm", ".css", ".map", ".js"}), "f2suffix", "replace tmpl file suffix")
	flag.Var(z.NewStrArr(&C.Front2.TmplPrefix, []string{"app.", "umi.", "runtime."}), "f2prefix", "replace tmpl file prefix")
	flag.BoolVar(&C.Front2.ChangeFile, "f2change", false, "change file when file change")

	z.Register("41-front2", func(zgg *z.Zgg) z.Closed {
		api := NewApi(www, C.Front2, "[_front2_]")
		// 增加路由
		zgg.AddRouter("", api.Serve)
		if C.Front2.ShowPath != "" {
			zgg.AddRouter("GET "+C.Front2.ShowPath, api.ListFile)
		}
		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})
}

func NewApi(www fs.FS, cfg Config, log string) *IndexApi {
	api := &IndexApi{LogKey: log, Config: cfg}
	if www != nil {
		api.FileFS, _ = GetFileMap(www)
		api.HttpFS = http.FS(www)
		if cfg.IsNative {
			api.ServeFS = http.FileServer(api.HttpFS)
		}
	}
	// 按字符串长度倒序
	api.RouterKey = []string{}
	for kk := range api.Config.Routers {
		api.RouterKey = append(api.RouterKey, kk)
	}
	slices.SortFunc(api.RouterKey, func(l string, r string) int { return -len(l) + len(r) })
	api.IndexsKey = []string{}
	for kk := range api.Config.Indexs {
		api.IndexsKey = append(api.IndexsKey, kk)
	}
	slices.SortFunc(api.IndexsKey, func(l string, r string) int { return -len(l) + len(r) })
	// 输出日志
	if log != "" {
		z.Println(api.LogKey+":  indexs", api.IndexsKey)
		z.Println(api.LogKey+": routers", api.RouterKey)
	}
	return api
}

type IndexApi struct {
	LogKey    string
	Config    Config
	IndexsKey []string
	HttpFS    http.FileSystem // 文件系统, http.FS(wwwFS)
	FileFS    map[string]fs.FileInfo
	RouterMap map[string]http.Handler
	RouterKey []string
	_svc_lock sync.RWMutex
	ServeFS   http.Handler // 直接服务, 优先级高，用于自定义配置
}

func (aa *IndexApi) GetProxy(kk string) http.Handler {
	if aa.RouterMap == nil {
		return nil
	}
	aa._svc_lock.RLock()
	defer aa._svc_lock.RUnlock()
	return aa.RouterMap[kk]
}

func (aa *IndexApi) NewProxy(kk string) (http.Handler, error) {
	aa._svc_lock.Lock()
	defer aa._svc_lock.Unlock()
	vv := aa.Config.Routers[kk]
	proxy, err := gtw.NewTargetProxy(vv) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa.RouterMap == nil {
		aa.RouterMap = make(map[string]http.Handler)
	}
	aa.RouterMap[kk] = proxy
	return proxy, nil
}

// Serve
func (aa *IndexApi) Serve(zrc *z.Ctx) {
	aa.ServeHTTP(zrc.Writer, zrc.Request)
}

// ServeHTTP
// 这里为什么选择 List 作为路由遍历， 而不是 TrieTree ？
// 1. 优先选择 List 遍历的场景
// 路由数量少（如 ≤20 个）：List 遍历的性能更高，实现更简单。
// 路由规则简单：无动态参数、通配符或前缀匹配需求。
// 对性能要求极高：如高频请求的边缘服务或嵌入式系统。
// 2. 优先选择 TrieTree 的场景
// 路由数量多（如 ≥50 个）：TrieTree 的时间复杂度为 O(k)，性能优势明显。
// 路由规则复杂：需要支持动态参数、通配符或前缀匹配。
// 路由频繁更新：TrieTree 的插入和删除操作效率更高（O(k) vs O(n)）
func (aa *IndexApi) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	// 后端路由代理
	for _, kk := range aa.RouterKey {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if kk == rr.URL.Path {
			// 确定验证文件， 如果是验证文件， 直接返回 vv 内容
			if vv := aa.Config.Routers[kk]; strings.HasPrefix(vv, "@") {
				z.WriteRespBytes(rw, "text/plain; charset=utf-8", http.StatusOK, []byte(vv[1:]))
				// http.ServeContent(rw, rr, "", time.Now(), bytes.NewReader([]byte(vv)[1:]))
				return
			}
		}
		if z.IsDebug() {
			z.Printf(aa.LogKey+": %s[%s] -> %s\n", kk, rr.URL.Path, aa.Config.Routers[kk])
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk); err != nil {
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		} else {
			proxy.ServeHTTP(rw, rr) // next
		}
		return
	}
	// 一个特殊接口， 解决 cdn 场景下， base url 动态识别问题， 默认返回 /， 基于 Referer 识别
	// 由于该接口执行在 Router 之后，所以可以通过 Router 配置，来屏蔽该接口
	if rr.URL.Path == "_getbasepath.txt" {
		referer := rr.URL.Query().Get("referer") // query 参数优先
		if referer == "" {
			referer = rr.Referer() // header 参数备选
		}
		var rerr error
		basepath := ""
		if referer == "" {
			rerr = errors.New("no referer")
		} else if refurl, err := url.Parse(referer); err != nil {
			rerr = errors.New("parse referer error: " + err.Error())
		} else {
			rr.URL.Path = refurl.Path // 替换请求路径， 使用工具函数处理
			basepath = FixReqPath(rr, aa.IndexsKey, "")
		}
		if rerr != nil {
			z.Println(aa.LogKey+":", "_getbasepath.txt error,", rerr.Error())
		}
		if basepath == "" {
			basepath = "/" // 默认根路径
		}
		z.WriteRespBytes(rw, "text/plain; charset=utf-8", http.StatusOK, []byte(basepath))
		// http.ServeContent(rw, rr, "", time.Now(), bytes.NewReader([]byte(basepath)))
		return
	}
	// --------------------------------------------------------------
	// 前端资源文件访问
	rp := FixReqPath(rr, aa.IndexsKey, "")
	if z.IsDebug() {
		z.Printf(aa.LogKey+": { path: '%s', raw: '%s', root: '%s'}\n", rr.URL.Path, rr.URL.RawPath, rp)
	}
	if aa.ServeFS != nil {
		rr.Header.Set("X-Req-RootPath", rp) // 标记请求根路径
		aa.ServeFS.ServeHTTP(rw, rr)
		return
	}
	if aa.HttpFS == nil {
		http.Error(rw, "404 Not Found", http.StatusNotFound)
		return
	}
	if aa.Config.ChangeFile {
		aa.ChgIndexContent(rw, rr, rp)
	} else {
		aa.TryIndexContent(rw, rr, rp)
	}
}
