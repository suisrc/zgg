package front2

import (
	"bytes"
	"net/http"
	"path/filepath"

	"github.com/suisrc/zgg/z"
)

// TryIndexContent，依赖FileFS，不支持文件变动
func (aa *IndexApi) TryIndexContent(rw http.ResponseWriter, rr *http.Request, rp string) {
	fpath := rr.URL.Path
	if fpath == "" {
		fpath = "/"
	}
	_, exist := aa.FileFS[fpath[1:]]
	if !exist {
		// 确定是否有文件后缀，如果有文件后缀，直接返回 404
		if ext := filepath.Ext(fpath); ext != "" {
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
		http.Error(rw, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
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
