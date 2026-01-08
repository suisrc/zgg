// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"net/http"

	"github.com/suisrc/zgg/ze/gtw"
)

// 鉴权器, 一个简单的例子， 需要扩展后才可使用
func NewAuthorize2(sites []string, client AuthClient) *Authorize2 {
	return &Authorize2{
		Authorize0: gtw.NewAuthorize0(sites),
		client:     client,
		CookieKey:  "kat",
	}
}

var _ gtw.Authorizer = (*Authorize2)(nil)

// 通过接口验证权限
type Authorize2 struct {
	gtw.Authorize0
	CookieKey string     // cookie key
	client    AuthClient // 请求客户端
}

// *redis.Client = redis.NewClient(*redis.Options)
// *redis.ClusterClient = redis.NewClusterClient(*redis.ClusterOptions)

func (aa *Authorize2) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt gtw.IRecord) bool {
	aa.Authorize0.Authz(gw, rw, rr, rt)
	cid, err := rr.Cookie(aa.CookieKey)
	if err != nil {
		// 没有登录信息，直接返回 401 错误
		rw.WriteHeader(http.StatusUnauthorized)
		return false
	}
	auth, err := aa.client.Do(rw, rr, cid.Value)
	if err != nil {
		if err != gtw.ErrNil {
			// gw.GetErrorHandler()(rw, rr, err) // StatusBadGateway = 502
			msg := "error in authorize2, get userinfo, " + err.Error()
			gw.Logf(msg + "\n")
			rw.WriteHeader(http.StatusInternalServerError)
			if rt != nil {
				rt.SetRespBody([]byte("###" + msg))
			}
		}
		return false
	}
	// 添加请求头
	rr.Header.Set("X-Request-Sky-Authorize", auth)
	return true
}
