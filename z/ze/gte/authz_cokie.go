// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"net/http"

	"github.com/suisrc/zgg/z/ze/gtw"
)

// 鉴权器, 一个简单的例子， 需要扩展后才可使用
func NewAuthzCokie(sites []string, client AuthzClient) gtw.Authorizer {
	return &AuthzCokie{
		AuthRecord: gtw.NewAuthRecord(sites),
		client:     client,
		CookieKey:  "kat",
	}
}

// var _ gtw.Authorizer = (*AuthzCookie)(nil)

type AuthzCokie struct {
	gtw.AuthRecord
	client    AuthzClient // 请求客户端
	CookieKey string      // cookie key
}

// *redis.Client = redis.NewClient(*redis.Options)
// *redis.ClusterClient = redis.NewClusterClient(*redis.ClusterOptions)

func (aa *AuthzCokie) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt gtw.IRecord) bool {
	aa.AuthRecord.Authz(gw, rw, rr, rt)
	// 处理 cookie 内容
	cid, err := rr.Cookie(aa.CookieKey)
	if err != nil {
		// 没有登录信息，直接返回 401 错误
		rw.WriteHeader(http.StatusUnauthorized)
		return false
	}
	rst, err := aa.client.Do(rw, rr, aa.CookieKey, cid.Value)
	if err != nil {
		if err != gtw.ErrNil {
			// gw.GetErrorHandler()(rw, rr, err) // StatusBadGateway = 502
			msg := "error in authzcokie, get userinfo, " + err.Error()
			gw.Logf(msg + "\n")
			rw.WriteHeader(http.StatusInternalServerError)
			if rt != nil {
				rt.SetRespBody([]byte("###" + msg))
			}
		}
		return false
	}
	// rr.Header.Set("X-Request-Sky-Authorize", auth)
	return rst
}
