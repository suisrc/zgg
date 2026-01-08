// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 用于扩展的鉴权系统, 这个文件仅做参考

package gte

import (
	"net/http"

	"github.com/suisrc/zgg/ze/gtw"
)

// 只记录日志， 不进行鉴权
func NewLoggerOnly(sites []string) *gtw.Authorize0 {
	gw := gtw.NewAuthorize0(sites)
	return &gw
}

type AuthClient interface {
	Do(rw http.ResponseWriter, req *http.Request, key string) (string, error)
}

// -----------------------------------------------------------------------------------------

// 鉴权器, basic auth， 基础鉴权， 仅限于参考
func NewAuthorize0(sites []string, client AuthClient) *Authorize2 {
	return &Authorize2{
		Authorize0: gtw.NewAuthorize0(sites),
		client:     client,
	}
}

var _ gtw.Authorizer = (*Authorize0)(nil)

// 通过接口验证权限
type Authorize0 struct {
	gtw.Authorize0
	client AuthClient // 请求客户端
}

func (aa *Authorize0) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt gtw.IRecord) bool {
	aa.Authorize0.Authz(gw, rw, rr, rt)
	//----------------------------------------
	user, pwd, ok := rr.BasicAuth()
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return false
	}
	rid, err := aa.client.Do(rw, rr, user)
	if err != nil {
		if err != gtw.ErrNil {
			// gw.GetErrorHandler()(rw, rr, err) // StatusBadGateway = 502
			msg := "error in authorize0, get password by name, " + err.Error()
			gw.Logf(msg + "\n")
			rw.WriteHeader(http.StatusInternalServerError)
			if rt != nil {
				rt.SetRespBody("###" + msg)
			}
		}
		return false
	}
	if pwd != rid {
		rw.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}
