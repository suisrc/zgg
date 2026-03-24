package front2

import (
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/suisrc/zgg/z"
)

// 特殊标记处理函数
func (aa *IndexApi) ServeAction(rw http.ResponseWriter, rr *http.Request, rc string) bool {
	if len(rc) < 2 {
		return false // 非特殊标记 | 路径不匹配
	} else if kk := rc[2:]; !z.HasPathPrefix(rr.URL.Path, kk) {
		return false // 非特殊标记 | 路径不匹配
	} else if fn, ok := aa.Actions[rc[:2]]; !ok {
		return false // 非特殊标记 | 操作不存在
	} else if vv, _ := aa.Config.Routers[rc]; vv == "" {
		return false // 非特殊标记 | 路由不存在
	} else {
		return fn(aa, rw, rr, kk, vv)
	}
}

// 操作方法
type ActionFunc func(aa *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool

// 操作列表， 可以扩展
var ActionOpts = map[string]ActionFunc{
	"@:": func(api *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool {
		// 扩展请求头上的信息, 增加路由标记 KEY
		rr.Header.Set("X-Req-RouteKey", vv)
		return false // 不终止请求， 继续后面的业务请求
	},
	"@@": func(aa *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool {
		// 确定验证文件，要求 路径完成相同, 否则跳过，路径不一致，使用后面的 @ 标记
		if kk == rr.URL.Path {
			z.WriteRespBytes(rw, "text/plain; charset=utf-8", http.StatusOK, []byte(vv))
		} else {
			http.Error(rw, "404 Path Not Match,", http.StatusNotFound)
		}
		return true // 终止服务
	},
	"@#": func(aa *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool {
		// @# 开头，返回格式 xxx[#(code,)content-type)]
		var data string
		var code int = http.StatusOK
		var ctyp string = "text/plain; charset=utf-8"
		if data1, code1, ok := strings.Cut(vv, "#"); ok {
			data = data1
			if code2, ctyp2, ok := strings.Cut(code1, ","); ok {
				code1, ctyp = code2, ctyp2
			}
			if stt, _ := strconv.Atoi(code1); stt >= 200 && stt < 600 {
				code = stt
			}
		} else {
			data = vv
		}
		data = strings.ReplaceAll(data, "{{rid}}", z.GetTraceID(rr))
		z.WriteRespBytes(rw, ctyp, code, []byte(data))
		return true // 终止服务
		// http.ServeContent(rw, rr, "", time.Now(), bytes.NewReader([]byte(vv)[1:]))
	},
	"@>": func(aa *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool {
		// 路由重定向
		if strings.HasSuffix(rr.URL.Path, "/_getbasepath") {
			return false // 路由重定向 不处理 _getbasepath 请求
		}
		if strings.HasPrefix(vv, "~") {
			// 跳转到新的路径, 隐式重定向，地址不变，服务不变，直接修改 rr.URL.Path， 内部重定向
			rr.URL.Path, rr.URL.RawPath = vv[1:], ""
			return false // 不终止请求， 继续后面的业务请求
		} else {
			// 跳转到新的路径, 显式重定向，使用 303 跳转， 注意 301 重定向是永久重定向，不再此范畴内
			http.Redirect(rw, rr, vv, http.StatusSeeOther)
			return true // 终止服务
		}
	},
	"@^": func(aa *IndexApi, rw http.ResponseWriter, rr *http.Request, kk, vv string) bool {
		// 请求重定向
		if strings.HasSuffix(rr.URL.Path, "/_getbasepath") {
			return false // 请求重定向 不处理 _getbasepath 请求
		}
		//  不建议这样使用，建议使用 router 直接路由。这只是一种特殊的路由方式方式， 增加 首页判断， 为CDN提供支持
		apath, rpath := aa.GetRootPath(rr)
		if z.IsDebug() {
			z.Printf(aa.LogKey+": { path: '%s', raw: '%s', root: '%s'}\n", rr.URL.Path, rr.URL.RawPath, rpath)
		}
		if strings.HasPrefix(vv, "~") {
			// 只支持 GET 请求， 适合 CDN 加载的索引文件
			target := strings.TrimSuffix(vv[1:], "/")
			path := target + apath
			resp, err := http.Get(path)
			if err != nil {
				z.Println(aa.LogKey+": error, redirect to:", path, rr.URL.Path, err.Error())
				http.Redirect(rw, rr, path, http.StatusMovedPermanently)
			} else {
				if ctype := resp.Header.Get("Content-Type"); strings.HasPrefix(ctype, "application/octet-stream") {
					resp.Header.Set("Content-Type", "text/html; charset=utf-8")
				}
				if rpath != "" {
					rw.Header().Set("X-Request-Rp", rpath) // 通过 CDN 加载的索引文件，存在 /rootpath 未替换的问题
					// X-Request-Rp 与 X-Req-RootPath 区分, 防止被意外替换, X-Req-RootPath 来自上级路由业务的内容
				}
				maps.Copy(rw.Header(), resp.Header)
				rw.WriteHeader(resp.StatusCode)
				io.Copy(rw, resp.Body)
			}
		} else {
			// 支持所有的请求， 底层逻辑和 Routers 相同
			rr.URL.Path, rr.URL.RawPath = apath, ""
			target := strings.TrimSuffix(vv, "/") // 目标地址， 支持 /~/ 和 /-/ 语法
			if proxy := aa.GetProxy(target); proxy != nil {
				proxy.ServeHTTP(rw, rr) // next
			} else if proxy, err := aa.NewProxy(target, target); err != nil {
				http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
			} else {
				proxy.ServeHTTP(rw, rr) // next
			}
		}
		return true // 终止服务
	},
}
