package front2

// 请求会扩展 Header 信息
// X-Req-RootPath: 识别/匹配 的根目录信息
// X-Req-RouteKey: 路由中的非定向指定，而是路由 Key 信息
// Routers: 路由特殊规则说明， 以 @ 开头会激活路由的处理模式
//    @= 全匹配模式， 要求 key=URL.Path, 返回值Content-Type: text/plain; charset=utf-8
//    @: 请求头标记， 会在请求头 X-Req-RouteKey 增加标记， 便于后面路由处理
//    @> 路由重定向， @>http/@>~ 使用 303 重定向路由地址, 否则修改路由的 URL.Path，为指定的路由
//    @^ 请求重定向， @^~ 使用 router 模式，否认使用 request 模式
//    @... 其他，格式为： @xxx[#code(,content-type)] 完全之定义返回的内容，可使用 {{rid}} 参数

import (
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
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
	api.ActionKey = []string{}
	for kk, vv := range api.Config.Routers {
		if strings.HasPrefix(vv, "@") {
			api.ActionKey = append(api.ActionKey, kk)
		} else {
			api.RouterKey = append(api.RouterKey, kk)
		}
	}
	if len(api.RouterKey) > 1 {
		slices.SortFunc(api.RouterKey, func(l string, r string) int { return -len(l) + len(r) })
	}
	if len(api.ActionKey) > 1 {
		slices.SortFunc(api.ActionKey, func(l string, r string) int { return -len(l) + len(r) })
	}
	// 首页索引
	api.IndexsKey = []string{}
	for kk := range api.Config.Indexs {
		api.IndexsKey = append(api.IndexsKey, kk)
	}
	if len(api.IndexsKey) > 1 {
		slices.SortFunc(api.IndexsKey, func(l string, r string) int { return -len(l) + len(r) })
	}
	// 输出日志
	if log != "" {
		z.Println(api.LogKey+": routers", api.RouterKey)
		z.Println(api.LogKey+": actions", api.ActionKey)
		z.Println(api.LogKey+": indexes", api.IndexsKey)
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
	ActionKey []string // 路由的特殊标记， X-Req-RouteKey， 必须是以@开头
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

func (aa *IndexApi) NewProxy(kk, vv string) (http.Handler, error) {
	aa._svc_lock.Lock()
	defer aa._svc_lock.Unlock()
	if vv == "" {
		vv = aa.Config.Routers[kk]
		if vv == "" {
			return nil, fmt.Errorf("router not found: %s", kk)
		}
	}
	var proxy http.Handler
	var err error
	if strings.HasPrefix(vv, "domain+") {
		proxy, err = gtw.NewDomainProxy(vv[7:], "") // 创建目标URL
	} else {
		proxy, err = gtw.NewTargetProxy(vv) // 创建目标URL
	}
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
	if aa.ServeRouter(rw, rr) {
		return
	}
	// 特殊操作代理
	if aa.ServeAction(rw, rr) {
		return
	}
	// --------------------------------------------------------------
	// 前端资源文件识别
	rp := FixReqPath(rr, aa.IndexsKey, "")
	if z.IsDebug() {
		z.Printf(aa.LogKey+": { path: '%s', raw: '%s', root: '%s'}\n", rr.URL.Path, rr.URL.RawPath, rp)
	}
	// 代理文件系统访问
	if aa.ServeFS != nil {
		rr.Header.Set("X-Req-RootPath", rp) // 标记请求根路径
		aa.ServeFS.ServeHTTP(rw, rr)
		return
	}
	// 当前文件系统访问
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

// 获取 rootpath 路径
func (aa *IndexApi) GetRootPath(rr *http.Request) (string, string) {
	apath := ""
	rpath := FixReqPath(rr, aa.IndexsKey, "")
	if ext := filepath.Ext(rr.URL.Path); ext != "" {
		apath = rr.URL.Path // 文件资源
	} else {
		if rpath != "" {
			// 寻找指定索引文件
			apath, _ = aa.Config.Indexs[rpath]
		} else {
			// 通过匹配查询索引文件
			for _, kk := range aa.IndexsKey {
				if rr.URL.Path == kk || zc.HasPrefixFold(rr.URL.Path, kk+"/") {
					apath = aa.Config.Indexs[kk] // 匹配到, 使用 v 代替 index
					break
				}
			}
		}
	}
	if apath == "" {
		apath = aa.Config.Index
	}
	if !strings.HasPrefix(apath, "/") {
		apath = "/" + apath
	}
	return apath, rpath
}
