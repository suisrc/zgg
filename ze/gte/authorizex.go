package gte

import (
	"net/http"

	"github.com/suisrc/zgg/ze/gtw"
)

var _ gtw.Authorizer = (*gtw.Authorize0)(nil)

// 全通关，不验证
type Authorizex struct {
	gtw.Authorize0
	AuthzServe string // 验证服务器
}

// 不适用指针，是方便其他依赖注入
func NewAuthorizex(sites []string, authz string) *Authorizex {
	return &Authorizex{
		Authorize0: gtw.NewAuthorize0(sites),
	}
}

func (aa *Authorizex) Authz(rw http.ResponseWriter, req *http.Request, rec *gtw.RecordTrace) bool {
	aa.Authorize0.Authz(rw, req, rec)
	// 确定是否有权限， 如果没有权限，直接返回结果，如果有权限，继续后续访问

	return true
}
