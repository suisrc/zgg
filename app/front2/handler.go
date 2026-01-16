package front2

import (
	"bytes"
	"flag"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
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
	Folder   string            `json:"folder"`   // 文件系统文件夹， 比如 /www, 必须是 / 开头
	ShowPath string            `json:"f2show"`   // 显示 www 文件夹资源
	IsNative bool              `json:"native"`   // 使用原生文件服务
	RootPath []string          `json:"rootpath"` // 访问根目录, 访问根目录， 删除根目录后才是文件目录
	Index    string            `json:"index"`    // 默认首页文件名, index.html
	Indexs   map[string]string `json:"indexs"`   // index map, 用于多个 index 系统中，不同 rootpath 对应不同的 index.html
	Routers  map[string]string `json:"routers"`  // 路由表
	TmplPath string            `json:"tmpl"`     // 模版根目录, ROOT_PATH, 构建时可以在运行时替换，用于静态资源路径替换
	TmplSuff []string          `json:"suff"`     // 替换文件后缀, .html .htm .css .map .js
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *IndexApi, zgg *z.Zgg)

func Init3(www fs.FS, dir string, ifn InitializFunc) {
	z.Config(&C)

	flag.Var(z.NewStrVal(&C.Front2.Folder, dir), "f2folder", "static folder")
	flag.StringVar(&C.Front2.ShowPath, "f2show", "", "show www resource uri")
	flag.BoolVar(&C.Front2.IsNative, "native", false, "use native file server")
	flag.Var(z.NewStrArr(&C.Front2.RootPath, []string{"/zgg", "/demo1/demo2"}), "f2rp", "root dir parts list")
	flag.StringVar(&C.Front2.Index, "index", "index.html", "index file name")
	flag.Var(z.NewStrMap(&C.Front2.Indexs, z.HM{"/zgg": "index.htm"}), "indexs", "index file name")
	flag.Var(z.NewStrMap(&C.Front2.Routers, z.HM{"/api1/": "http://127.0.0.1:8081/api2/"}), "f2rmap", "router path replace")
	flag.StringVar(&C.Front2.TmplPath, "tmpl", "ROOT_PATH", "root router path")
	flag.Var(z.NewStrArr(&C.Front2.TmplSuff, []string{".html", ".htm", ".css", ".map", ".js"}), "suff", "replace tmpl file suffix")

	z.Register("41-front2", func(zgg *z.Zgg) z.Closed {
		hfs := http.FS(www)
		api := &IndexApi{Config: C.Front2, HttpFS: hfs}
		if C.Front2.IsNative {
			api.ServeFS = http.FileServer(hfs)
		}
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

	ServeFS   http.Handler    // 文件服务, 优先级高，存在优先使用，不存使用HttpFS弥补
	HttpFS    http.FileSystem // 文件系统, http.FS(wwwFS)
	_end_rmap map[string]http.Handler
	_end_lock sync.RWMutex
}

func (aa *IndexApi) GetProxy(kk string) http.Handler {
	if aa._end_rmap == nil {
		return nil
	}
	aa._end_lock.RLock()
	defer aa._end_lock.RUnlock()
	return aa._end_rmap[kk]
}

func (aa *IndexApi) NewProxy(kk, vv string) (http.Handler, error) {
	aa._end_lock.Lock()
	defer aa._end_lock.Unlock()
	proxy, err := gtw.NewTargetProxy(vv) // 创建目标URL
	if err != nil {
		return nil, err
	}
	if aa._end_rmap == nil {
		aa._end_rmap = make(map[string]http.Handler)
	}
	aa._end_rmap[kk] = proxy
	return proxy, nil
}

// ServeHTTP
func (aa *IndexApi) ServeHTTP(zrc *z.Ctx) {
	rw := zrc.Writer
	rr := zrc.Request
	for kk, vv := range aa.Config.Routers {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if z.IsDebug() {
			z.Printf("[_routing]: %s[%s] -> %s\n", kk, rr.URL.Path, vv)
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk, vv); err != nil {
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		} else {
			proxy.ServeHTTP(rw, rr) // next
		}
		return
	}
	// --------------------------------------------------------------
	rp := FixPath(rr, aa.Config.RootPath, aa.Config.Folder)
	if z.IsDebug() {
		z.Printf("[_request]: { path: '%s', raw: '%s', root: '%s'}\n", //
			rr.URL.Path, rr.URL.RawPath, rp)
	}
	if aa.ServeFS != nil {
		aa.ServeFS.ServeHTTP(rw, rr)
	} else {
		aa.TryIndex(rw, rr, rp)
	}
}

// 判断是否需要转换内容
func (aa *IndexApi) isFixCtx(name string) bool {
	if aa.Config.TmplPath == "" {
		return false
	}
	for _, suff := range aa.Config.TmplSuff {
		if strings.HasSuffix(name, suff) {
			return true
		}
	}
	return false

}

// 获取 index.html 文件名
func (aa *IndexApi) getIndex(rp string) string {
	if index, ok := aa.Config.Indexs[rp]; ok {
		return index
	}
	return aa.Config.Index
}

// try index
func (aa *IndexApi) TryIndex(rw http.ResponseWriter, rr *http.Request, rp string) {
	redirect := false
	filename := ""
	file, err := aa.HttpFS.Open(rr.URL.Path)
	if err != nil {
		redirect = true // 重定向到首页
	} else if stat, err := file.Stat(); err != nil {
		redirect = true // 重定向到首页
	} else if stat.IsDir() {
		// 尝试当前目录的 index.html 文件， 如果没有在重定向到根目录
		index := aa.getIndex(rp)
		rr.URL.Path = filepath.Join(rr.URL.Path, index)
		if rr.URL.RawPath != "" {
			rr.URL.RawPath = filepath.Join(rr.URL.RawPath, index)
		}
		aa.TryIndex(rw, rr, rp) // 重定向到当前路径的 index.html
		return
		// 直接重定向到根目录的 index.html
		// filename = "-" // 标记为文件夹, 直接跳转到 index.html
		// redirect = true // 重定向到首页
	} else if aa.isFixCtx(stat.Name()) {
		tbts, _ := io.ReadAll(file) // 需要转换内容 tp -> rp
		tbts = bytes.ReplaceAll(tbts, []byte("/"+aa.Config.TmplPath), []byte(rp))
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
	} else {
		// 正常返回文件
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
	}
	if file != nil {
		file.Close() // 释放文件
	}
	if redirect {
		if filename == "" {
			// 文件不存在， 如果有文件后缀，且后缀不是 .html, .html 则返回 404
			if idx := strings.LastIndex(rr.URL.Path, "/"); idx > 0 {
				filename = rr.URL.Path[idx+1:]
			} else {
				filename = rr.URL.Path
			}
			if idx := strings.LastIndex(filename, "."); idx < 0 {
				// 文件没有后缀，可能是文件夹，需要重定向到 index.html
			} else if suff := filename[idx:]; suff != ".html" && suff != ".htm" {
				http.NotFound(rw, rr)
				return // 后缀不是 .html, .html 则返回 404
			}
		}
		// 重定向到 index.html（支持前端路由的history模式）
		index := aa.getIndex(rp)
		ipath := filepath.Join(aa.Config.Folder, index)
		file, err = aa.HttpFS.Open(ipath)
		if err != nil {
			z.Printf("[_index__]: [%s] %s\n", ipath, err.Error())
			http.NotFound(rw, rr) // 没有重定向的 index.html 文件
			return
		}
		defer file.Close()
		stat, _ := file.Stat()
		if aa.isFixCtx(stat.Name()) {
			tbts, _ := io.ReadAll(file) // 需要转换内容 tp -> rp
			tbts = bytes.ReplaceAll(tbts, []byte("/"+aa.Config.TmplPath), []byte(rp))
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
		} else {
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
		}
	}
}

// 修正路径
func FixPath(rr *http.Request, paths []string, folder string) string {
	rp := ""
	for _, path := range paths {
		if strings.HasPrefix(rr.URL.Path, path) {
			rp = path
			rr.URL.Path = rr.URL.Path[len(rp):]
			rr.URL.RawPath = strings.TrimPrefix(rr.URL.RawPath, rp)
			break
		}
	}
	if folder != "" {
		rr.URL.Path = filepath.Join(folder, rr.URL.Path)
		if rr.URL.RawPath != "" {
			rr.URL.RawPath = filepath.Join(folder, rr.URL.RawPath)
		}
	}
	return rp
}
