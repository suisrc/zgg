package front2

import (
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/suisrc/zgg/z"
)

func (aa *IndexApi) ServeRouter(rw http.ResponseWriter, rr *http.Request) bool {
	for _, kk := range aa.RouterKey {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue
		}
		if z.IsDebug() {
			z.Printf(aa.LogKey+": %s[%s] -> %s\n", kk, rr.URL.Path, aa.Config.Routers[kk])
		}
		if proxy := aa.GetProxy(kk); proxy != nil {
			proxy.ServeHTTP(rw, rr) // next
		} else if proxy, err := aa.NewProxy(kk, ""); err != nil {
			http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		} else {
			proxy.ServeHTTP(rw, rr) // next
		}
		return true
	}
	return false
}

func (aa *IndexApi) ServeAction(rw http.ResponseWriter, rr *http.Request) bool {
	for _, kk := range aa.ActionKey {
		if !strings.HasPrefix(rr.URL.Path, kk) {
			continue // 非路由内容
		}
		vv := aa.Config.Routers[kk]
		if len(vv) < 2 {
			continue // 非特殊标记
		}
		switch vv[:2] {
		case "@=":
			// 确定验证文件，要求 路径完成相同, 否则跳过
			if kk == rr.URL.Path {
				z.WriteRespBytes(rw, "text/plain; charset=utf-8", http.StatusOK, []byte(vv[2:]))
			} else {
				http.Error(rw, "404 Path Not Match,", http.StatusNotFound)
			}
			return true // 终止服务
		case "@:":
			// 扩展请求头上的信息, 增加路由标记 KEY
			rr.Header.Set("X-Req-RouteKey", vv[2:])
			// 不终止请求， 继续后面的业务请求
		case "@>":
			if strings.HasPrefix(vv, "@>~") {
				// 路径重定向， 跳转到新的路径, 显式重定向，使用 303 跳转， 注意 301 重定向是永久重定向，不再此范畴内
				http.Redirect(rw, rr, vv[3:], http.StatusSeeOther)
				return true // 终止服务
			} else if strings.HasPrefix(vv, "@>http") {
				http.Redirect(rw, rr, vv[2:], http.StatusSeeOther)
				return true // 终止服务
			} else {
				// 路径重定向， 跳转到新的路径, 隐式重定向，地址不变，服务不变，直接修改 rr.URL.Path， 内部重定向
				rr.URL.Path, rr.URL.RawPath = vv[2:], ""
				// 不终止请求， 继续后面的业务请求
			}
		case "@^":
			// 请求重定向， 不建议这样使用，建议使用 router 直接路由。这只是一种特殊的路由方式
			apath, rpath := aa.GetRootPath(rr)
			if z.IsDebug() {
				z.Printf(aa.LogKey+": { path: '%s', raw: '%s', root: '%s'}\n", rr.URL.Path, rr.URL.RawPath, rpath)
			}
			if strings.HasPrefix(vv, "@^~") {
				rr.URL.Path, rr.URL.RawPath = apath, ""
				target := strings.TrimSuffix(vv[3:], "/")
				if proxy := aa.GetProxy(target); proxy != nil {
					proxy.ServeHTTP(rw, rr) // next
				} else if proxy, err := aa.NewProxy(target, target); err != nil {
					http.Error(rw, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
				} else {
					proxy.ServeHTTP(rw, rr) // next
				}
			} else {
				target := strings.TrimSuffix(vv[2:], "/")
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
			}
			return true // 终止服务
		default:
			if strings.HasPrefix(vv, "@@") {
				vv = vv[1:] // 跳过特殊标记， 用于解决包含 @=， @:， @$, @&， @~ 等特殊情况
			}
			// @开头，在 RouterXey 中已经标记，这里不做特殊处理， 不建议这么配置，为了兼容之前的旧系统配置
			// 置顶语返回格式，格式 @xxx[#code(,content-type)]
			vs := strings.SplitN(vv[1:], "#", 2)
			code := http.StatusOK
			var ctype string
			if len(vs) != 2 {
				// 格式 @xxx 使用 200,text/plain 默认方式响应
				ctype = "text/plain; charset=utf-8"
			} else if cts := strings.SplitN(vs[1], ",", 2); len(cts) != 2 {
				// 格式 @xxx#code 使用 text/plain 默认方式响应
				if stt, _ := strconv.Atoi(vs[1]); stt >= 200 && stt < 600 {
					code = stt
				}
				ctype = "text/plain; charset=utf-8"
			} else {
				// 格式 @xxx#code,content-type 完全自定义相应内容
				if status, _ := strconv.Atoi(cts[0]); status >= 200 && status < 600 {
					code = status
				}
				ctype = cts[1]
			}
			data := strings.ReplaceAll(vs[0], "{{rid}}", z.GetTraceID(rr))
			z.WriteRespBytes(rw, ctype, code, []byte(data))
			return true // 终止服务
			// http.ServeContent(rw, rr, "", time.Now(), bytes.NewReader([]byte(vv)[1:]))
		}
	}
	// 一个特殊接口， 解决 cdn 场景下， base url 动态识别问题， 默认返回 /， 优先基于 Referer 识别
	// 由于该接口执行在 Router 之后，所以可以通过 Router 配置，来屏蔽该接口
	if strings.HasSuffix(rr.URL.Path, "/_getbasepath") {
		if referer := rr.Referer(); referer == "" {
			// z.Println(aa.LogKey+": (", rr.URL.Path, ") referer is empty,")
		} else if refurl, err := url.Parse(referer); err != nil {
			z.Println(aa.LogKey+": (", rr.URL.Path, ") parse referer error,", err.Error())
		} else {
			rr.URL.Path = refurl.Path // 替换请求路径， 使用工具函数处理
		}
		basepath := FixReqPath(rr, aa.IndexsKey, "")
		if basepath == "" {
			basepath = "/" // 默认根路径
		}
		if accept := rr.Header.Get("Accept"); accept != "" && strings.HasPrefix(accept, "application/json") {
			// HasPrefix or Contains
			data := `{"success":true,"data":"` + basepath + `"}`
			z.WriteRespBytes(rw, "application/json; charset=utf-8", http.StatusOK, []byte(data))
		} else {
			z.WriteRespBytes(rw, "text/plain; charset=utf-8", http.StatusOK, []byte(basepath))
		}
		// http.ServeContent(rw, rr, "", time.Now(), bytes.NewReader([]byte(basepath)))
		return true
	}
	return false
}
