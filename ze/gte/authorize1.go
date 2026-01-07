// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/suisrc/zgg/ze/gtw"
)

// 鉴权器， 为 f1kin 系统定制的验证器
// authz 鉴权服务器，authz = "" 时，只记录日志，不进行鉴权
// askip 请求中存在 X-Request-Sky-Authorize，可以忽略鉴权
func NewAuthorize1(sites []string, authz string, askip bool) *Authorize1 {
	return &Authorize1{
		Authorize0: gtw.NewAuthorize0(sites),
		AuthzServe: authz,
		AllowSkipz: askip,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: gtw.TransportGtw0,
		},
	}
}

var _ gtw.Authorizer = (*Authorize1)(nil)

// 通过接口验证权限
type Authorize1 struct {
	gtw.Authorize0
	AuthzServe string       // 验证服务器
	AllowSkipz bool         // 允许跳过验证
	client     *http.Client // 请求客户端
}

func (aa *Authorize1) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt gtw.IRecord) bool {
	aa.Authorize0.Authz(gw, rw, rr, rt)
	if rt != nil {
		rt.SetSrvAuthz(aa.AuthzServe)
	}
	if aa.AuthzServe == "" {
		return true // 只记录日志，不进行鉴权， 同 NewLoggerAuthz
	}
	if aa.AllowSkipz && rr.Header.Get("X-Request-Sky-Authorize") != "" {
		return true // 忽略验证
	}
	//----------------------------------------
	// if ainfo := rr.Header.Get("X-Request-Sky-Authorize"); ainfo != "" {
	// 	return true // 已验证 ？？？
	// }
	ctx, cancel := context.WithTimeout(rr.Context(), 3*time.Second)
	defer cancel() // 验证需要在 3s 完成，以防止后面业务阻塞
	// 处理 url
	uri := aa.AuthzServe
	if rr.URL.RawQuery != "" {
		// 增加查询参数
		if idx := strings.IndexRune(uri, '?'); idx > 0 {
			uri += "&"
		} else {
			uri += "?"
		}
		uri += rr.URL.RawQuery
	}
	if _, err := url.Parse(uri); err != nil {
		msg := "error in authorize1, parse zuthz addr, " + err.Error()
		gw.Logf(msg + "\n")
		rw.WriteHeader(http.StatusInternalServerError)
		if rt != nil {
			rt.SetRespBody("###" + msg)
		}
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aa.AuthzServe, nil)
	if err != nil {
		msg := "error in authorize1, new request context, " + err.Error()
		gw.Logf(msg + "\n")
		rw.WriteHeader(http.StatusInternalServerError)
		if rt != nil {
			rt.SetRespBody("###" + msg)
		}
		return false
	}
	// 处理 header
	gtw.CopyHeader(req.Header, rr.Header)
	req.Header.Set("X-Request-Origin-Host", rr.Host)
	req.Header.Set("X-Request-Origin-Path", rr.URL.Path)
	req.Header.Set("X-Request-Origin-Method", rr.Method)
	req.Header.Set("X-Request-Origin-Action", gtw.GetAction(rr.URL))
	// 强制要求返回用户信息，所以在拷贝 header 时候，需要过滤 "X-Request-Sky-Authorize"
	req.Header.Set("X-Debug-Force-User", "961212") // 日志需要登录人信息
	// 请求远程鉴权服务器
	resp, err := aa.client.Do(req)
	if err != nil {
		gw.GetErrorHandler()(rw, req, err)
		if rt != nil {
			rt.SetRespBody("###error authorize1, request authz serve, " + err.Error())
		}
		return false
	}
	defer resp.Body.Close()
	// X- 开头的 header 传递给 rr
	for kk, vv := range resp.Header {
		if gtw.HasPrefixFold(kk, "X-") {
			rr.Header[kk] = vv
		}
	}
	// 处理结果
	if resp.StatusCode >= 300 || resp.Header.Get("X-Request-Sky-Authorize") == "" {
		// 验证失败，返回结果
		body, _ := io.ReadAll(resp.Body)
		// 记录返回日志
		rt.LogOutRequest(rr) // 带有的请求信息，用于记录
		rt.LogResponse(resp) // 带有的响应信息，用于记录
		rt.LogRespBody(int64(len(body)), nil, body)
		// 认证结果返回
		dst := rw.Header()
		// 过滤 X-Request- , 其他的传递给 rw
		for k, vv := range resp.Header {
			if gtw.HasPrefixFold(k, "X-Request-") {
				continue // 忽略用户信息
				// X-Debug-Force-User 会触发 X-Request-Sky-Authorize 强制返回
			}
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		rw.Write(body)
		return false
	}

	// 验证成功，继续后续访问, Set-Cookie 传递给 rw
	if sc, ok := rw.Header()["Set-Cookie"]; ok {
		for _, v := range sc {
			rw.Header().Add("Set-Cookie", v)
		}
	}
	return true
}
