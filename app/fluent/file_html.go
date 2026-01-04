package fluent

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
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
func (aa *FluentApi) lst(zrc *z.Ctx) {
	rw := zrc.Writer
	rr := zrc.Request

	// query参数，path: 文件路径
	queryPath := rr.URL.Query().Get("path")
	if strings.Contains(queryPath, "..") {
		rw.WriteHeader(http.StatusForbidden)
		rw.Write([]byte("Forbidden"))
		return
	} else if queryPath == "" {
		queryPath = "/"
	}
	// 兑换为 http fs 系统的文件
	httpFile, err := aa.HttpFS.Open(queryPath)
	if err != nil {
		// 文件读取发生异常
		http.NotFound(rw, rr)
		rw.Write([]byte(err.Error()))
		return
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
		dirPaths := []fs.FileInfo{} // 文件夹
		filPaths := []fs.FileInfo{} // 文件
		for _, path := range pathList {
			if path.IsDir() {
				dirPaths = append(dirPaths, path)
			} else {
				filPaths = append(filPaths, path)
			}
		}
		sort.Slice(dirPaths, func(i, j int) bool {
			return dirPaths[i].Name() < dirPaths[j].Name()
		})
		sort.Slice(filPaths, func(i, j int) bool {
			return filPaths[i].Name() < filPaths[j].Name()
		})
		// ----------------------------------------------------
		var html_body strings.Builder
		parentPath := filepath.Dir(queryPath)
		if parentPath == "/" {
			parentPath = ""
		}
		if queryPath == "/" {
			queryPath = ""
		}
		// ----------------------------------------------------
		fmt.Fprintf(&html_body, "<a href=\"%s?path=%s\">../</a>\n", aa.RoutePath, parentPath)
		for _, path := range dirPaths {
			name := path.Name() + "/"
			fmt.Fprintf(&html_body, "<a href=\"%s?path=%s/%s\">%s</a>\n", //
				aa.RoutePath, queryPath, path.Name(), name)
		}
		for _, path := range filPaths {
			name := path.Name()
			fmt.Fprintf(&html_body, "<a href=\"%s?path=%s/%s\">%s</a>\n", //
				aa.RoutePath, queryPath, path.Name(), name)
		}
		// 整合列表到 html 中
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Write([]byte(html_top + html_body.String() + thml_end))
	}
}
