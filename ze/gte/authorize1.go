package gte

// 通过远程 authz 接口， 获取用于是否登录权限

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/suisrc/zgg/ze/gtw"
)

var _ gtw.Authorizer = (*Authorize1)(nil)

// 通过接口验证权限
type Authorize1 struct {
	gtw.Authorize0
	AuthzServe string       // 验证服务器
	client     *http.Client // 请求客户端
}

// 鉴权器
func NewAuthorize1(sites []string, authz string) *Authorize1 {
	return &Authorize1{
		Authorize0: gtw.NewAuthorize0(sites),
		AuthzServe: authz,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: gtw.TransportGtw0,
		},
	}
}

func (aa *Authorize1) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt *gtw.RecordTrace) bool {
	aa.Authorize0.Authz(gw, rw, rr, rt)
	// 确定是否有权限， 如果没有权限，直接返回结果，如果有权限，继续后续访问
	// 请求是完成独立的，所以，不需要使用 rr.Context()
	// if ainfo := rr.Header.Get("X-Request-Sky-Authorize"); ainfo != "" {
	// 	return true // 已验证 ？？？
	// }
	ctx, cancel := context.WithTimeout(rr.Context(), 3*time.Second)
	defer cancel() // 验证需要在 3s 完成，以防止后面业务阻塞
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aa.AuthzServe, nil)
	if err != nil {
		gw.GetErrorHandler()(rw, req, err)
		if rt != nil {
			rt.RespBody = []byte("###error authorizex, " + err.Error())
		}
		return false
	}
	gtw.CopyHeader(req.Header, rr.Header)
	req.Header.Set("X-Request-Origin-Host", rr.Host)
	req.Header.Set("X-Request-Origin-Path", rr.URL.Path)
	req.Header.Set("X-Request-Origin-Method", rr.Method)
	req.Header.Set("X-Request-Origin-Action", rr.URL.Query().Get("action"))
	// 强制要求返回用户信息，所以在拷贝 header 时候，需要过滤 "X-Request-Sky-Authorize"
	req.Header.Set("X-Debug-Force-User", "961212") // 测试用
	// 请求远程鉴权服务器
	resp, err := aa.client.Do(req)
	if err != nil {
		gw.GetErrorHandler()(rw, req, err)
		if rt != nil {
			rt.RespBody = []byte("###error authorizex, " + err.Error())
		}
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// 验证失败，返回结果
		body, _ := io.ReadAll(resp.Body)
		rt.LogResponse(resp)
		rt.LogRespBody(int64(len(body)), nil, body)

		dst := rw.Header()
		for k, vv := range resp.Header {
			if gtw.EqualFold(k, "X-Request-Sky-Authorize") {
				continue // 忽略用户信息，内容由于 X-Debug-Force-User 导致
			}
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		rw.Write(body)
		return false
	}
	// 验证成功，继续后续访问
	// X- 开头的 header 传递给 rr， Set-Cookie 开头的传递给 rw
	for kk, vv := range resp.Header {
		if gtw.HasPrefixFold(kk, "X-") {
			rr.Header[kk] = vv
		} else if gtw.EqualFold(kk, "Set-Cookie") {
			for _, v := range vv {
				rw.Header().Add(kk, v)
			}
		}
	}
	return true
}
