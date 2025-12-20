package front2

import (
	"embed"
	"flag"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/cfg"
)

var (
	C = struct {
		Front2 Front2Config
	}{}
)

type Front2Config struct {
	IsNative bool   `json:"native"`
	DirParts string `json:"dirs"`
	Index    string `json:"index"`
	Indexs   string `json:"indexs"`
	TmplPath string `json:"tmpl"`
	TmplSuff string `json:"suff"`
}

func Init(efs embed.FS) {
	cfg.Register(&C)

	flag.StringVar(&C.Front2.DirParts, "dirs", "/zgg,/demo1/demo2", "root dir parts list")
	flag.StringVar(&C.Front2.TmplPath, "tmpl", "ROOT_PATH", "root router path")
	flag.StringVar(&C.Front2.TmplSuff, "suff", ".html,.htm,.css,.map,.js", "replace tmpl file suffix")
	flag.StringVar(&C.Front2.Index, "index", "index.html", "index file name")
	flag.StringVar(&C.Front2.Indexs, "indexs", "/zgg=index.htm", "index file name")
	flag.BoolVar(&C.Front2.IsNative, "native", false, "use native file server")

	z.Register("10-www", func(srv z.IServer) z.Closed {
		hfs := http.FS(efs)
		api := &IndexApi{
			DirParts: []string{},
			TmplPath: C.Front2.TmplPath,
			TmplSuff: strings.Split(C.Front2.TmplSuff, ","),
			Index_:   C.Front2.Index,
			Indexs:   map[string]string{},
			Folder:   "/www",
			HttpFS:   hfs,
		}
		if C.Front2.DirParts != "" {
			api.DirParts = strings.Split(C.Front2.DirParts, ",")
		}
		if C.Front2.Indexs != "" {
			for v := range strings.SplitSeq(C.Front2.Indexs, ",") {
				kv := strings.Split(v, "=")
				if len(kv) == 2 {
					api.Indexs[kv[0]] = kv[1]
				}
			}
		}
		if C.Front2.IsNative {
			api.ServeFS = http.FileServer(hfs)
		}
		srv.AddRouter("", api.ServeFile)
		return nil
	})
}

type IndexApi struct {
	DirParts []string          // 访问根目录, 访问根目录， 删除根目录后才是文件目录
	TmplPath string            // 模版根目录, ROOT_PATH, 构建时可以在运行时替换，用于静态资源路径替换
	TmplSuff []string          // 替换文件后缀, .html .htm .css .map .js
	Index_   string            // 默认首页文件名, index.html
	Indexs   map[string]string // index map, 用于多个 index 系统中，不同 rootpath 对应不同的 index.html
	Folder   string            // 文件系统文件夹， 比如 /www, 必须是 / 开头
	HttpFS   http.FileSystem   // 文件系统, http.FS(wwwFS)
	ServeFS  http.Handler      // 文件服务, 优先级高，存在优先使用，不存使用HttpFS弥补
}

func (aa *IndexApi) ServeFile(zrc *z.Ctx) bool {
	rp := FixPath(zrc.Request, aa.DirParts, aa.Folder)
	if z.C.Debug {
		z.Printf("[_request]: { path: '%s', raw: '%s', root: '%s'}\n", //
			zrc.Request.URL.Path, zrc.Request.URL.RawPath, rp)
	}
	if aa.ServeFS != nil {
		aa.ServeFS.ServeHTTP(zrc.Writer, zrc.Request)
	} else {
		aa.TryIndex(zrc.Writer, zrc.Request, rp)
	}
	return true
}

// 判断是否需要转换内容
func (aa *IndexApi) isFixCtx(name string) bool {
	if aa.TmplPath == "" {
		return false
	}
	for _, suff := range aa.TmplSuff {
		if strings.HasSuffix(name, suff) {
			return true
		}
	}
	return false

}

// 获取 index.html 文件名
func (aa *IndexApi) getIndex(rp string) string {
	index := aa.Index_
	if ipage, ok := aa.Indexs[rp]; ok {
		index = ipage
	}
	return index
}

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
		// 需要转换内容 C.RootRout -> C.RootPath
		text, _ := io.ReadAll(file)
		tstr := strings.ReplaceAll(string(text), "/"+aa.TmplPath, rp)
		trdr := strings.NewReader(tstr)
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), trdr)
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
		ipath := aa.Folder + "/" + index
		file, err = aa.HttpFS.Open(ipath)
		if err != nil {
			z.Printf("[_index__]: [%s] %s\n", ipath, err.Error())
			http.NotFound(rw, rr) // 没有重定向的 index.html 文件
			return
		}
		defer file.Close()
		stat, _ := file.Stat()
		if !aa.isFixCtx(stat.Name()) {
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
			// rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			// idata, _ := io.ReadAll(ifile)
			// rw.Write(idata)
		}
		// 需要转换内容 C.RootRout -> C.RootPath
		text, _ := io.ReadAll(file)
		tstr := strings.ReplaceAll(string(text), "/"+aa.TmplPath, rp)
		trdr := strings.NewReader(tstr)
		http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), trdr)
	}
}

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
		rr.URL.Path = folder + rr.URL.Path
		if rr.URL.RawPath != "" {
			rr.URL.RawPath = folder + rr.URL.RawPath
		}
	}
	return rp
}
