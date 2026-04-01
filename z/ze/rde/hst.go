package rde

import (
	"net/http"
	"strings"

	"github.com/suisrc/zgg/z"
)

// -----------------------------------------------------------------------------------
// hst 识别 host，让后在进行 rdx 路由识别， 路由样式 http://{hst}.<domain>/...

// var _ z.Engine = (*HstRouter)(nil)

func NewHstRouter(svckit z.SvcKit) z.Engine {
	helper, _ := svckit.Get("rde-helper").(Helper)
	return &HstRouter{
		name:   "zgg-hst",
		svckit: svckit,
		Helper: helper,
		Router: make(map[string]z.Engine),
	}
}

type HstRouter struct {
	name   string
	svckit z.SvcKit
	Helper Helper
	Router map[string]z.Engine
}

func (aa *HstRouter) Name() string {
	return aa.name
}

func (aa *HstRouter) Handle(method, action string, handle z.HandleFunc) {
	if aa.Helper == nil {
		// 如果 Helper 为空，直接返回，不注册任何路由
		return
	}
	if z.C.Server.ApiPath != "" {
		action = strings.TrimPrefix("/"+action, z.C.Server.ApiPath+"/")
	}
	// 分拆 action 为 key 和 path
	key, path := "", ""
	if idx := strings.Index(action, "/"); idx > 0 {
		key, path = action[:idx], action[idx+1:]
	} else {
		key, path = action, ""
	}
	// 获取路由是否存在，不存在则创建一个新的路由并注册到 aa.Router 中，最后在路由上注册 handle
	router, exist := aa.Router[key]
	if !exist {
		router = aa.Helper.NewRouter(aa.svckit)
		aa.Router[key] = router
	}
	router.Handle(method, path, handle)
}

func (aa *HstRouter) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if aa.Helper == nil {
		// 如果 Helper 为空，直接返回 404 Not Found
		http.NotFound(rw, rr)
		return
	}
	if z.C.Server.ApiPath != "" {
		rr.URL.Path = strings.TrimPrefix(rr.URL.Path, z.C.Server.ApiPath+"/")
		rr.URL.RawPath = strings.TrimPrefix(rr.URL.RawPath, z.C.Server.ApiPath+"/")
	}
	// 从 Host 中提取子域名作为 key
	host := rr.Host
	if idx := strings.Index(host, "."); idx > 0 {
		key, err := aa.Helper.KeyGetter(host[:idx])
		if err == nil {
			if router, exist := aa.Router[key]; exist {
				rr.Header.Set("X-Router-Key", host[:idx]) // 设置 X-Router-Key 头部信息
				router.ServeHTTP(rw, rr)
				return
			}
		} else if err != z.IngoreErr {
			// 其他错误，返回 500 Internal Server Error
			http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	// 没有匹配到任何路由，返回 404 Not Found
	http.NotFound(rw, rr)
}

// SetRouterKeyByHst 从 Host 中提取子域名作为 key，并设置到 X-Router-Key 头部信息中，供后续路由识别使用
func SetRouterKeyByHst(rr *http.Request) {
	if rr == nil {
		return
	}
	host := rr.Host
	if idx := strings.Index(host, "."); idx > 0 {
		rkey := host[:idx]
		rr.Header.Set("X-Router-Key", rkey) // 设置 X-Router-Key 头部信息
	}
}
