package fluent

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/suisrc/zgg/z"
)

var (
	html_top = `
<html><head><title>LOGS</title></head>
<body>
<h1>Logs File List /</h1><hr><pre>

`
	thml_end = `
</pre><hr></body></html>`
)

// 列表文件
func (aa *FluentApi) lst(zrc *z.Ctx) bool {
	rw := zrc.Writer
	rr := zrc.Request

	// query参数，path: 文件路径
	queryPath := rr.URL.Query().Get("path")
	if strings.Contains(queryPath, "..") {
		rw.WriteHeader(http.StatusForbidden)
		rw.Write([]byte("Forbidden"))
		return true
	} else if queryPath == "" {
		queryPath = "/"
	}
	// 兑换为 http fs 系统的文件
	httpFile, err := aa.HttpFS.Open(queryPath)
	if err != nil {
		// 文件读取发生异常
		http.NotFound(rw, rr)
		rw.Write([]byte(err.Error()))
		return true
	}
	defer httpFile.Close() // 退出时候关闭文件
	// 确定文件状态 ========================================================
	if httpStat, err := httpFile.Stat(); err != nil {
		// 读取文件信息发生异常
		http.NotFound(rw, rr)
		rw.Write([]byte(err.Error()))
	} else if !httpStat.IsDir() {
		// 资源是一个文件，直接写出
		http.ServeContent(rw, rr, httpStat.Name(), httpStat.ModTime(), httpFile)
	} else if pathList, err := httpFile.Readdir(-1); err != nil {
		// 读取文件列表出现异常
		http.NotFound(rw, rr)
		rw.Write([]byte(err.Error()))
	} else {
		var html_body strings.Builder
		// <a href="../">../</a>
		parentPath := filepath.Dir(queryPath)
		if parentPath == "/" {
			parentPath = ""
		}
		if queryPath == "/" {
			queryPath = ""
		}
		fmt.Fprintf(&html_body, "<a href=\"%s?path=%s\">../</a>\n", aa.RoutePath, parentPath)
		for _, path := range pathList {
			name := path.Name()
			if path.IsDir() {
				name = name + "/"
			}
			fmt.Fprintf(&html_body, "<a href=\"%s?path=%s/%s\">%s</a>\n", //
				aa.RoutePath, queryPath, path.Name(), name)
		}
		// 整合列表到 html 中
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Write([]byte(html_top + html_body.String() + thml_end))
	}
	return true
}
