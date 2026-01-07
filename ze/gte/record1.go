// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gte

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/gtw"
)

// 日志内容

var _ gtw.FRecord = (*Record1)(nil)

type Record1 struct {
	Record0

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
}

// ==========================================================

func (rc Record1) MarshalJSON() ([]byte, error) {
	return zc.ToJsonBytes(&rc, "json", zc.LowerFirst, false)
}

func (rc *Record1) ToJson() ([]byte, error) {
	return zc.ToJsonBytes(rc, "json", zc.LowerFirst, false)
}

func (rc *Record1) ToStr() string {
	return zc.ToStr(&rc)
}

func (rc *Record1) ToFmt() string {
	return zc.ToStr2(&rc)
}

// ==========================================================

// Convert by RecordTrace
func ToRecord1(rt_ gtw.IRecord) gtw.FRecord {
	rt, _ := rt_.(*gtw.Record0)
	rc := &Record1{}
	ByRecord0(rt, &rc.Record0)

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

	// -------------------------------------------------------------------
	if rc.Result2 != "success" || rc.RespBody == nil {
		// 请求异常或响应体为空，PASS
	} else if data, ok := rc.RespBody.(map[string]any); ok {
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
	return rc
}
