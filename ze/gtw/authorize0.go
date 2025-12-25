package gtw

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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
		ClientKey: "_zc",
		SiteHosts: sites,
		ClientAge: 2 * 365 * 24 * 3600, // 默认2年
	}
}

func (aa *Authorize0) Authz(rw http.ResponseWriter, req *http.Request, rec *RecordTrace) bool {
	if aa.ClientKey == "" {
		return true
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if cid, err := req.Cookie(aa.ClientKey); err == nil {
		// client id 存在
		if rec != nil {
			rec.ClientID = cid.Value
		}
		// 如果时间小于1/8，续签
		if idx := strings.LastIndexByte(cid.Value, '.'); idx <= 0 || idx == len(cid.Value)-1 {
		} else if ctc, err := strconv.Atoi(cid.Value[idx+1:]); err != nil {
		} else if time.Now().Unix()-int64(ctc) > int64(aa.ClientAge-aa.ClientAge/8) {
			// 续签
			siteHost := ""
			for _, host := range aa.SiteHosts {
				if strings.HasSuffix(req.Host, host) {
					siteHost = host
					break
				}
			}
			hck := &http.Cookie{
				Name:     aa.ClientKey,
				Value:    cid.Value,
				MaxAge:   aa.ClientAge,
				Path:     "",
				Domain:   siteHost,
				SameSite: http.SameSiteDefaultMode,
				Secure:   false,
				HttpOnly: false,
			}
			http.SetCookie(rw, hck)
		}
		return true
	}
	// 新增
	pre := aa.ClientKey
	if pre[0] == '_' {
		pre = pre[1:]
	}
	clientID := fmt.Sprintf("%s.1.00.%s.%d", aa.ClientKey, GenStr("", 16), time.Now().Unix())
	siteHost := ""
	for _, host := range aa.SiteHosts {
		if strings.HasSuffix(req.Host, host) {
			siteHost = host
			break
		}
	}
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
	req.AddCookie(cookie)
	if rec != nil {
		rec.ClientID = clientID
	}

	return true
}
