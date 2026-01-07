// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gtw"
)

var SensitiveRequestHeaders = []string{
	"Authorization",
	"Cookie",
	"X-Request-Sky-",
}

// 日志基础内容

var _ gtw.FRecord = (*Record0)(nil)

type Record0 struct {
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
	RespBody any
	RespSize int64
	Result2  string
}

// ==========================================================

func (rc Record0) MarshalJSON() ([]byte, error) {
	return zc.ToJsonBytes(&rc, "json", zc.LowerFirst, false)
}

func (rc *Record0) ToJson() ([]byte, error) {
	return zc.ToJsonBytes(rc, "json", zc.LowerFirst, false)
}

func (rc *Record0) ToStr() string {
	return zc.ToStr(&rc)
}

func (rc *Record0) ToFmt() string {
	return zc.ToStr2(&rc)
}

// ==========================================================

// Convert by RecordTrace
func ToRecord0(rt_ gtw.IRecord) gtw.FRecord {
	rt, _ := rt_.(*gtw.Record0)
	rc := &Record0{}
	ByRecord0(rt, rc)
	return rc
}
func ByRecord0(rt *gtw.Record0, rc *Record0) {
	rc.GatewayName = gtw.GetServeName()

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
		rc.Action = gtw.GetAction(uri)
	}

	rc.ServiceName = rt.ReqHost
	rc.ServiceAddr = rt.UpstreamAddr
	rc.ServiceAuth = rt.SrvAuthzAddr

	rc.Requester = rc.RemoteAddr
	if strings.HasPrefix(rc.Requester, "127.0.0.1") {
		// 请求者是自己？本地调试或者正向代理， 标志当前节点名称即可
		rc.Requester = rc.GatewayName + "/" + rc.Requester
	}
	rc.Responder = rc.ServiceAddr
	if strings.HasPrefix(rc.Responder, "127.0.0.1") {
		// 接受者是自己， kwdog 鉴权系统拦截，需要标记服务名为节点
		_, port, _ := net.SplitHostPort(rc.ServiceAddr)
		rc.ServiceAddr = gtw.GetLocAreaIp()
		if port != "" {
			rc.ServiceAddr = rc.ServiceAddr + ":" + port
		}
		rc.Responder = rc.GatewayName + "/" + rc.Responder
		rc.ServiceName = rc.GatewayName
	}
	// 清除多余后缀，注意， statefulset 是 全面，pod 和 deployment 非全名
	// rc.ServiceName = strings.TrimSuffix(rc.ServiceName, ".cluster.local")
	// -------------------------------------------------------------------
	// 请求
	if rt.OutReqHeader != nil {
		for kk, vv := range rt.OutReqHeader {
			sensitive := false
			for _, ss := range SensitiveRequestHeaders {
				if gtw.HasPrefixFold(kk, ss) {
					sensitive = true
					break
				}
			}
			if sensitive {
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
	// 相应头信息
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
	// 响应体信息尝试解析
	if len(rt.RespBody) > 0 {
		if rt.RespBody[0] == '{' {
			// json 响应体，解析内容， 如果解析失败，跳过，这里需要消耗大量资源
			map_ := map[string]any{}
			if err := json.Unmarshal(rt.RespBody, &map_); err == nil {
				rc.RespBody = map_
			}
		}
		if rc.RespBody == nil {
			rc.RespBody = string(rt.RespBody)
		}
	}
	rc.RespSize = rt.RespSize
	// -------------------------------------------------------------------
	// 尝试分析结果
	rc.Result2 = "success"
	if rc.Status >= 400 {
		rc.Result2 = "abnormal" // 异常
	} else if rc.Status >= 300 {
		rc.Result2 = "redirect" // 重定向
	}
}
