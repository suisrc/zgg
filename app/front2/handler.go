package front2

import (
	"bytes"
	"flag"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gtw"
)

var (
	C = struct {
		Front2 Front2Config
	}{}
)

type Front2Config struct {
	ShowPath   string            `json:"f2show"`  // 显示 www 文件夹资源
	IsNative   bool              `json:"native"`  // 使用原生文件服务
	Index      string            `json:"index"`   // 默认首页文件名, index.html
	Indexs     map[string]string `json:"indexs"`  // index map, 用于多个 index 系统中，不同 rootpath 对应不同的 index.html
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
	flag.Var(z.NewStrMap(&C.Front2.Routers, z.HM{"/api1/": "http://127.0.0.1:8081/api2/"}), "f2rmap", "router path replace")
	flag.StringVar(&C.Front2.TmplRoot, "f2trpath", "/ROOT_PATH", "root path, empty is disabled")
	flag.Var(z.NewStrArr(&C.Front2.TmplSuffix, []string{".html", ".htm", ".css", ".map", ".js"}), "f2suffix", "replace tmpl file suffix")
	flag.Var(z.NewStrArr(&C.Front2.TmplPrefix, []string{"app.", "umi.", "runtime."}), "f2prefix", "replace tmpl file prefix")
	flag.BoolVar(&C.Front2.ChangeFile, "f2change", false, "change file when file change")

	z.Register("41-front2", func(zgg *z.Zgg) z.Closed {
		hfs := http.FS(www)
		api := &IndexApi{Config: C.Front2, HttpFS: hfs}
		if C.Front2.IsNative {
			api.ServeFS = http.FileServer(hfs)
		}
		api.FileFS, _ = GetFileMap(www)
		// 按字符串长度倒序
		for kk := range api.Config.Routers {
			api.RoutersKey = append(api.RoutersKey, kk)
		}
		slices.SortFunc(api.RoutersKey, func(l string, r string) int { return -len(l) + len(r) })
		z.Println("[_front2_]: routers", api.RoutersKey)
		for kk := range api.Config.Indexs {
			api.IndexsKey = append(api.IndexsKey, kk)
		}
		slices.SortFunc(api.IndexsKey, func(l string, r string) int { return -len(l) + len(r) })

		// 增加路由
		zgg.AddRouter("", api.ServeHTTP)
		if C.Front2.ShowPath != "" {
			zgg.AddRouter("GET "+C.Front2.ShowPath, api.ListFile)
		}

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

type IndexApi struct {
	Config Front2Config

	ServeFS    http.Handler    // 文件服务, 优先级高，存在优先使用，不存使用HttpFS弥补
	HttpFS     http.FileSystem // 文件系统, http.FS(wwwFS)
	FileFS     map[string]fs.FileInfo
	IndexsKey  []string
	RoutersEnd map[string]http.Handler
	RoutersKey []string
	_end_lock  sync.RWMutex
}

func (aa *IndexApi) GetProxy(kk string) http.Handler {
	if aa.RoutersEnd == nil {
		return nil
	}
	aa._end_lock.RLock()
	defer aa._end_lock.RUnlock()
	return aa.RoutersEnd[kk]
}

func (aa *IndexApi) NewProxy(kk string) (http.Handler, error) {
	aa._end_lock.Lock()
	defer aa._end_lock.Unlock()
	vv := aa.Config.Routers[kk]
	proxy, err := gtw.NewTargetProxy(vv) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa.RoutersEnd == nil {
		aa.RoutersEnd = make(map[string]http.Handler)
	}
	aa.RoutersEnd[kk] = proxy
	return proxy, nil
}

// ServeHTTP
func (aa *IndexApi) ServeHTTP(zrc *z.Ctx) {
	rw := zrc.Writer
	rr := zrc.Request
	for _, kk := range aa.RoutersKey {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if z.IsDebug() {
			vv := aa.Config.Routers[kk]
			z.Printf("[_front2_]: %s[%s] -> %s\n", kk, rr.URL.Path, vv)
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
	// --------------------------------------------------------------
	rp := FixReqPath(rr, aa.IndexsKey, "")
	if z.IsDebug() {
		z.Printf("[_front2_]: { path: '%s', raw: '%s', root: '%s'}\n", rr.URL.Path, rr.URL.RawPath, rp)
	}
	if aa.ServeFS != nil {
		aa.ServeFS.ServeHTTP(rw, rr)
	} else if aa.Config.ChangeFile {
		aa.ChgIndex(rw, rr, rp)
	} else {
		aa.TryIndex(rw, rr, rp)
	}
}

// try index，依赖FileFS，不支持文件变动
func (aa *IndexApi) TryIndex(rw http.ResponseWriter, rr *http.Request, rp string) {
	fpath := rr.URL.Path
	if fpath == "" {
		fpath = "/"
	}
	_, exist := aa.FileFS[fpath[1:]]
	if !exist {
		// 确定是否有文件后缀，如果有文件后缀，直接返回 404
		if ext := filepath.Ext(fpath); ext != "" {
			z.Println("[_front2_]:", fpath, "file ext", ext)
			http.NotFound(rw, rr)
			return
		}
		// 重定向到 index.html（支持前端路由的history模式）
		fpath = aa.Config.Indexs[rp]
		if fpath == "" {
			fpath = aa.Config.Index
		}
		if len(fpath) > 0 && fpath[0] != '/' {
			fpath = "/" + fpath
		}
		_, exist = aa.FileFS[fpath[1:]]
	}
	// 文件不存在
	if !exist {
		http.NotFound(rw, rr)
		return
	}
	// 处理文件
	file, err := aa.HttpFS.Open(fpath)
	if err != nil {
		z.Printf("[_front2_]: [%s] %s\n", fpath, err.Error())
		http.NotFound(rw, rr)
		return // 没有重定向的 index.html 文件
	}
	defer file.Close()
	if stat, err := file.Stat(); err != nil {
		z.Printf("[_front2_]: [%s] %s\n", fpath, err.Error())
		http.Error(rw, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return // 读取文件信息错误
	} else if IsFixFile(stat.Name(), &aa.Config) {
		// 判断文件是否需要修复内容， 一般是依赖文件的引用问题
		tbts, err := GetFixFile(file, stat.Name(), aa.Config.TmplRoot, rp, aa.FileFS)
		if err != nil {
			z.Printf("[_front2_]: [%s] %s\n", fpath, err.Error())
			http.NotFound(rw, rr)
			return // 处理文件内容错误
		}
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
	} else {
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
	}
}

// chg index, 不依赖FileFS，支持文件变动
func (aa *IndexApi) ChgIndex(rw http.ResponseWriter, rr *http.Request, rp string) {
	redirect := false
	file, err := aa.HttpFS.Open(rr.URL.Path)
	if err != nil {
		redirect = true // 重定向到首页
	} else if stat, err := file.Stat(); err != nil {
		redirect = true // 重定向到首页
	} else if stat.IsDir() {
		redirect = true // 重定向到首页
	} else if IsFixFile(stat.Name(), &aa.Config) {
		// 文件的内容需要修复， 一般是依赖文件的引用问题
		tbts, err := GetFixFile(file, stat.Name(), aa.Config.TmplRoot, rp, aa.FileFS)
		if err != nil { // 内部异常
			z.Printf("[_front2_]: [%s] %s\n", rr.URL.Path, err.Error())
			http.Error(rw, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
	} else {
		// 正常返回文件
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
	}
	if file != nil {
		file.Close() // 释放文件
	}
	if redirect {
		// 确定是否有文件后缀，如果有文件后缀，直接返回 404
		if ext := filepath.Ext(rr.URL.Path); ext != "" {
			http.NotFound(rw, rr)
			return // 文件类型错误
		}
		// 重定向到 index.html（支持前端路由的history模式）
		index, _ := aa.Config.Indexs[rp]
		if index == "" {
			index = aa.Config.Index
		}
		file, err = aa.HttpFS.Open(index)
		if err != nil {
			z.Printf("[_front2_]: [%s] %s\n", index, err.Error())
			http.NotFound(rw, rr) // 没有重定向的 index.html 文件
			return
		}
		defer file.Close()
		if stat, err := file.Stat(); err != nil {
			z.Printf("[_front2_]: [%s] %s\n", index, err.Error())
			http.Error(rw, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		} else if IsFixFile(stat.Name(), &aa.Config) {
			tbts, err := GetFixFile(file, stat.Name(), aa.Config.TmplRoot, rp, aa.FileFS)
			if err != nil {
				z.Printf("[_front2_]: [%s] %s\n", index, err.Error())
				http.NotFound(rw, rr)
				return
			}
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
		} else {
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
		}
	}
}
