// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gtw

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Authorizer interface {
	Authz(gw IGateway, rw http.ResponseWriter, rr *http.Request, rt IRecord) bool
}

// -------------------------------------------------------------------------------------

var _ Authorizer = (*Authorize0)(nil)

// 全通关，不验证
type Authorize0 struct {
	ClientKey string   // Client ID key
	SiteHosts []string // 站点域名
	ClientAge int
}

// 不适用指针，是方便其他依赖注入
func NewAuthorize0(sites []string) Authorize0 {
	return Authorize0{
		ClientKey: "_xc",
		SiteHosts: sites,
		ClientAge: 2 * 365 * 24 * 3600, // 默认2年
	}
}

func (aa *Authorize0) Authz(gw IGateway, rw http.ResponseWriter, rr *http.Request, rt_ IRecord) bool {
	rt, _ := rt_.(*Record0) // 转换
	if rr.Header == nil {
		rr.Header = make(http.Header)
	}
	if rt != nil {
		// 添加 trace id
		if rt.TraceID == "" {
			rt.TraceID, _ = GenUUIDv4()
			rr.Header.Set("X-Request-Id", rt.TraceID)
		}
	}
	// ------------------------------------------------------------------
	if aa.ClientKey == "" {
		return true
	}
	// 强制读取一次, 用于记录日志
	cid, _ := rr.Cookie(aa.ClientKey)
	if rt != nil && cid != nil {
		rt.ClientID = cid.Value
	}
	// 更新和新增需要再可信站点上完成
	if len(aa.SiteHosts) == 0 {
		return true
	}
	siteHost := ""
	for _, host := range aa.SiteHosts {
		if strings.HasSuffix(rr.Host, host) {
			siteHost = host
			break
		}
	}
	if siteHost == "" {
		return true // 只处理已知的站点列表
	}
	if cid != nil {
		// 如果时间小于1/8，续签
		if idx := strings.LastIndexByte(cid.Value, '.'); idx <= 0 || idx == len(cid.Value)-1 {
			return true // 不同的版本，不创建，不续签
		} else if ctc, err := strconv.Atoi(cid.Value[idx+1:]); err != nil {
			return true // 不同的版本，不创建，不续签
		} else if time.Now().Unix()-int64(ctc) > int64(aa.ClientAge-aa.ClientAge/8) {
			cookie := &http.Cookie{
				Name:     aa.ClientKey,
				Value:    cid.Value,
				MaxAge:   aa.ClientAge,
				Path:     "",
				Domain:   siteHost,
				SameSite: http.SameSiteDefaultMode,
				Secure:   false,
				HttpOnly: false,
			}
			http.SetCookie(rw, cookie)
		}
	} else {
		// 新增
		pre := aa.ClientKey
		if pre[0] == '_' {
			pre = pre[1:]
		}
		clientID := fmt.Sprintf("%s.1.00.%s.%d", aa.ClientKey, GenStr("", 16), time.Now().Unix())
		// MaxAge=0 删除cookie
		// MaxAge<0 浏览器关闭
		// MaxAge>0 单位秒，当前时间叠加
		cookie := &http.Cookie{
			Name:     aa.ClientKey,
			Value:    url.QueryEscape(clientID),
			MaxAge:   aa.ClientAge,
			Path:     "",
			Domain:   siteHost,
			SameSite: http.SameSiteDefaultMode,
			Secure:   false,
			HttpOnly: false,
		}
		http.SetCookie(rw, cookie)
		rr.AddCookie(cookie)

		if rt != nil {
			rt.ClientID = clientID
			rt.Cookie[aa.ClientKey] = cookie
		}
	}

	return true
}
