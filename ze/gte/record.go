package gte

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/cfg"
	"github.com/suisrc/zgg/ze/gtw"
)

// 日志内容

type Record struct {
	TraceId   string
	ClientId  string
	RemoteIp  string
	UserAgent string
	Referer   string

	Scheme     string  // http scheme
	Host       string  // http host
	Path       string  // http path
	Method     string  // http method
	Status     int     // http status
	StartTime  string  // start time
	Action     string  // http action
	RemoteAddr string  // remote addr
	ReqTime    float64 // serve time
	Upstime    float64 // upstream time

	FlowId      string
	TokenId     string
	Nickname    string
	AccountCode string
	UserCode    string
	TenantCode  string
	UserTenCode string
	AppCode     string
	AppTenCode  string
	RoleCode    string

	MatchPolicys string

	ServiceName string
	ServiceAddr string
	ServiceAuth string
	GatewayName string

	Responder string
	Requester string

	ReqHeaders  string
	ReqHeader2s string
	RespHeaders string

	ReqBody  string
	RespBody string
	RespSize int64
	Result2  string
}

func (r Record) MarshalJSON() ([]byte, error) {
	return cfg.ToJsonBts(&r, "json", cfg.LowerFirst, false)
}

func (rc *Record) ToStr() string {
	return cfg.ToStr2(&rc)
}

func (rc *Record) ToFormatStr() string {
	return cfg.ToStr2(&rc)
}

// Convert by RecordTrace
func (rc *Record) ByRecord0(rt_ gtw.RecordTrace) {
	rt, _ := rt_.(*gtw.Record0)
	if rt == nil {
		return // 跳过
	}
	rc.GatewayName = GetLanDomain()

	rc.TraceId = rt.TraceID
	rc.RemoteIp = rt.RemoteIP
	rc.UserAgent = rt.UserAgent
	rc.Referer = rt.Referer
	rc.ClientId = rt.ClientID

	rc.Scheme = rt.Scheme
	rc.Host = rt.ReqHost
	rc.Path = rt.ReqURL
	rc.Method = rt.Method
	rc.Status = rt.StatusCode
	rc.StartTime = time.UnixMilli(rt.StartTime).Format("2006-01-02T15:04:05.000")
	rc.ReqTime = float64(rt.ServeTime) / 1000
	rc.Upstime = float64(rt.UpstreamTime) / 1000
	rc.RemoteAddr = rt.RemoteAddr

	// -------------------------------------------------------------------
	if uri, err := url.Parse(rt.ReqURL); err == nil {
		qry := uri.Query()
		rc.FlowId = qry.Get("flow")
		rc.Action = gtw.GetAction(uri)
	}
	if rt.OutReqHeader != nil {
		rc.MatchPolicys = rt.OutReqHeader.Get("X-Request-Sky-Policys")
	}
	if token := rt.OutReqHeader.Get("X-Request-Sky-Authorize"); token != "" {
		tknj := map[string]any{}
		if tknb, err := base64.StdEncoding.DecodeString(token); err != nil {
		} else if err := json.Unmarshal(tknb, &tknj); err != nil {
		} else {
			rc.TokenId, _ = tknj["jti"].(string)
			rc.Nickname, _ = tknj["nnm"].(string)
			rc.AccountCode, _ = tknj["sub"].(string)
			rc.UserCode, _ = tknj["uco"].(string)
			rc.TenantCode, _ = tknj["tco"].(string)
			rc.UserTenCode, _ = tknj["tuc"].(string)
			rc.AppCode, _ = tknj["three"].(string)
			rc.AppTenCode, _ = tknj["app"].(string)
			rc.RoleCode, _ = tknj["trc"].(string)
			if rc.RoleCode == "" {
				rc.RoleCode, _ = tknj["rol"].(string)
			}
		}
	}
	if rc.TokenId == "" {
		if auth := rt.OutReqHeader.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer kst.") {
			rc.TokenId = auth[52:76]
		} else if auth := rt.Cookie["kst"]; auth != nil && strings.HasPrefix(auth.Value, "kst.") {
			rc.TokenId = auth.Value[45:69]
		}
	}

	rc.ServiceName = rt.ReqHost
	rc.ServiceAddr = rt.UpstreamAddr
	rc.ServiceAuth = rt.SrvAuthzAddr

	rc.Requester = rc.RemoteAddr
	if strings.HasPrefix(rc.Requester, "127.0.0.1") {
		// 请求者是自己？本地调试或者正向代理， 标志当前节点名称即可
		rc.Requester = GetLanDomain() + "/" + rc.Requester
	}
	rc.Responder = rc.ServiceAddr
	if strings.HasPrefix(rc.Responder, "127.0.0.1") {
		// 接受者是自己， kwdog 鉴权系统拦截，需要标记服务名为节点
		_, port, _ := net.SplitHostPort(rc.ServiceAddr)
		rc.ServiceAddr = GetLocAreaIp()
		if port != "" {
			rc.ServiceAddr = rc.ServiceAddr + ":" + port
		}
		rc.Responder = rc.GatewayName + "/" + rc.ServiceAddr
		rc.ServiceName = rc.GatewayName
	}
	// 清除多余后缀，注意， statefulset 是 全面，pod 和 deployment 非全名
	rc.ServiceName = strings.TrimSuffix(rc.ServiceName, ".cluster.local")
	// -------------------------------------------------------------------
	// 请求
	if rt.OutReqHeader != nil {
		for kk, vv := range rt.OutReqHeader {
			if gtw.EqualFold(kk, "Authorization") || //
				gtw.EqualFold(kk, "Cookie") || //
				gtw.HasPrefixFold(kk, "X-Request-Sky-") {
				for _, v := range vv {
					rc.ReqHeader2s += kk + ": " + v + "\n"
				}
			} else {
				for _, v := range vv {
					rc.ReqHeaders += kk + ": " + v + "\n"
				}
			}

		}
	} else if rt.ReqHeader != nil {
		// OutReqHeader 包含 ReqHeader 的数据，所以 ReqHeaders 冗余
		for kk, vv := range rt.ReqHeader {
			for _, v := range vv {
				rc.ReqHeaders += kk + ": " + v + "\n"
			}
		}
	}
	if len(rt.ReqBody) > 0 {
		rc.ReqBody = string(rt.ReqBody)
	}
	// 相应
	if rt.RespHeader != nil {
		for kk, vv := range rt.RespHeader {
			for _, v := range vv {
				rc.RespHeaders += kk + ": " + v + "\n"
			}
			if gtw.EqualFold(kk, "X-Service-Name") && len(vv) > 0 {
				rc.Responder = vv[0]
			}
		}
	}
	if len(rt.RespBody) > 0 {
		rc.RespBody = string(rt.RespBody)
	}
	rc.RespSize = rt.RespSize
	// -------------------------------------------------------------------
	rc.Result2 = "success"
	if rc.Status >= 400 {
		rc.Result2 = "abnormal"
	} else if rc.Status >= 300 {
		rc.Result2 = "redirect"
	} else if len(rt.RespBody) > 0 && rt.RespBody[0] == '{' {
		// 响应体是 json, 尝试解析，确定响应结果
		data := map[string]any{}
		if err := json.Unmarshal(rt.RespBody, &data); err == nil {
			// 解析 json 响应体
			if succ, _ := data["success"]; succ != nil && !succ.(bool) {
				if showType, _ := data["showType"]; showType != nil {
					if showType.(int) == 9 {
						rc.Result2 = "redirect"
					} else {
						rc.Result2 = "abnormal"
					}
				} else if errshow, _ := data["errshow"]; errshow != nil {
					if errshow.(int) == 9 {
						rc.Result2 = "redirect"
					} else {
						rc.Result2 = "abnormal"
					}
				} else {
					rc.Result2 = "abnormal"
				}
			}
		}
	}

}

// -------------------------------------------------------------------
var (
	loc_areaip = ""
	lan_domain = ""
	namespace_ = ""
)

// 获取局域网地址
func GetLocAreaIp() string {
	if loc_areaip != "" {
		return loc_areaip
	}
	// 通过 /etc/hosts 获取局域网地址
	bts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		z.Printf("unable to read /etc/hosts: %s", err.Error())
	} else {
		for line := range strings.SplitSeq(string(bts), "\n") {
			if strings.HasPrefix(line, "#") {
				continue
			}
			ips := strings.Fields(line)
			// println("=============", cfg.ToStr(ips), len(ips))
			if len(ips) < 2 {
				continue
			}
			// 判断是否为 IPv4 地址
			if ip := net.ParseIP(strings.TrimSpace(ips[0])); ip == nil {
			} else if v4 := ip.To4(); v4 == nil {
			} else if v4.IsLoopback() {
			} else {
				loc_areaip = strings.TrimSpace(ips[0])
				lan_domain = strings.TrimSpace(ips[1])
				if !strings.ContainsRune(lan_domain, '.') {
					// 特殊情况，比如 pod 或者 deployment情况
					lan_domain += ".pod." + GetNamespace() + ".svc"
				}
				break
			}
		}
	}
	if loc_areaip == "" {
		loc_areaip = "127.0.0.1" // 无法解析
		lan_domain = "localhost"
	}

	return loc_areaip
}

// 获取局域网域名
func GetLanDomain() string {
	if lan_domain != "" {
		return lan_domain
	}
	GetLocAreaIp()
	return lan_domain
}

func GetNamespace() string {
	if namespace_ != "" {
		return namespace_
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		z.Printf("unable to read namespace: %s, return 'default'", err.Error())
		namespace_ = "-"
	} else {
		namespace_ = string(ns)
	}
	return namespace_
}
