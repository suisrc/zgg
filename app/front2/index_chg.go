package front2

import (
	"bytes"
	"net/http"
	"path/filepath"

	"github.com/suisrc/zgg/z"
)

// ChgIndexContent, 不依赖FileFS，支持文件变动
func (aa *IndexApi) ChgIndexContent(rw http.ResponseWriter, rr *http.Request, rp string) {
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
			z.Printf(aa.LogKey+": [%s] %s\n", rr.URL.Path, err.Error())
			http.Error(rw, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
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
			z.Printf(aa.LogKey+": [%s] %s\n", index, err.Error())
			http.NotFound(rw, rr) // 没有重定向的 index.html 文件
			return
		}
		defer file.Close()
		if stat, err := file.Stat(); err != nil {
			z.Printf(aa.LogKey+": [%s] %s\n", index, err.Error())
			http.Error(rw, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		} else if IsFixFile(stat.Name(), &aa.Config) {
			tbts, err := GetFixFile(file, stat.Name(), aa.Config.TmplRoot, rp, aa.FileFS)
			if err != nil {
				z.Printf(aa.LogKey+": [%s] %s\n", index, err.Error())
				http.NotFound(rw, rr)
				return
			}
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), bytes.NewReader(tbts))
		} else {
			http.ServeContent(rw, rr, stat.Name(), stat.ModTime(), file)
		}
	}
}
