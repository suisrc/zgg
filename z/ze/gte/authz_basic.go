// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"net/http"

	"github.com/suisrc/zgg/z/ze/gtw"
)

type AuthzClient interface {
	Do(rw http.ResponseWriter, req *http.Request, usr, pwd string) (bool, error)
}

// -----------------------------------------------------------------------------------------

// 只记录日志， 不进行鉴权
func NewAuthLogger(sites []string) gtw.Authorizer {
	az := gtw.NewAuthRecord(sites)
	return &az
}

// -----------------------------------------------------------------------------------------

// 鉴权器, basic authz 基础鉴权， 仅限于参考
func NewAuthzBasic(sites []string, client AuthzClient) gtw.Authorizer {
	return &AuthzBasic{
		AuthRecord: gtw.NewAuthRecord(sites),
		client:     client,
	}
}

// 通过接口验证权限
type AuthzBasic struct {
	gtw.AuthRecord
	client AuthzClient // 请求客户端
}

func (aa *AuthzBasic) Authz(gw gtw.IGateway, rw http.ResponseWriter, rr *http.Request, rt gtw.IRecord) bool {
	aa.AuthRecord.Authz(gw, rw, rr, rt)
	// 完成基础验证
	usr, pwd, ok := rr.BasicAuth()
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return false
	}
	rst, err := aa.client.Do(rw, rr, usr, pwd)
	if err != nil {
		if err != gtw.ErrNil {
			// gw.GetErrorHandler()(rw, rr, err) // StatusBadGateway = 502
			msg := "error in authzbasic, get password by name, " + err.Error()
			gw.Logf(msg + "\n")
			rw.WriteHeader(http.StatusInternalServerError)
			if rt != nil {
				rt.SetRespBody([]byte("###" + msg))
			}
		}
		return false
	}
	// rw.WriteHeader(http.StatusUnauthorized)
	return rst
}
