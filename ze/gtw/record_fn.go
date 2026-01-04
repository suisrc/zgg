// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gtw

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

// 记录原始请求内容
func (t *Record0) LogRequest(req *http.Request) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	t.StartTime = time.Now().UnixMilli()

	// trace id
	t.TraceID = req.Header.Get("X-Request-Id")
	t.RemoteIP = GetRemoteIP(req)
	t.UserAgent = req.UserAgent()
	t.Referer = req.Referer()

	t.Scheme = req.Header.Get("X-Scheme")
	t.Method = req.Method
	t.ReqHost = req.Host
	t.ReqURL = req.URL.String()
	t.ReqHeader = req.Header.Clone()

	for _, v := range req.Cookies() {
		t.Cookie[v.Name] = v
	}
	t.RemoteAddr = req.RemoteAddr

	if req.Method == http.MethodGet || req.Body == nil || //
		req.ContentLength == 0 || req.Header == nil {
		return // Issue 16036: nil Body for http.Transport retries
	}
	ct := t.ReqHeader.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") &&
		!strings.HasPrefix(ct, "application/xml") {
		t.RespBody = []byte("###request content type: " + ct)
		return // ignore
	}
	// 请求 body 大于 1MB， 不记录
	if req.ContentLength > 1024*1024 {
		t.ReqBody = []byte("###request body too large, skip")
	}
	// 输入的请求参数，必须记录， 输出的结果，根据结果大小，选择记录， 默认64KB
	t.ReqBody, _ = io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(t.ReqBody))
}

// 记录代理请求内容
func (t *Record0) LogOutRequest(outreq *http.Request) {
	t.OutReqHost = outreq.Host
	t.OutReqURL = outreq.URL.String()
	t.OutReqHeader = outreq.Header.Clone() // record request header
}

// 记录请求结果
func (t *Record0) LogResponse(res *http.Response) {
	t.UpstreamTime = time.Now().UnixMilli() - t.StartTime // 毫秒
	t.RespHeader = res.Header.Clone()
	t.StatusCode = res.StatusCode
	if res.StatusCode == http.StatusSwitchingProtocols {
		t.RespBody = []byte("###response body is websocket, skip")
		go t._track() // 提前记录 websocket 请求内容
	}

}

// 记录请求body内容
func (t *Record0) LogRespBody(bsz int64, err error, buf []byte) {
	t.RespSize = bsz
	if t.RespHeader == nil || bsz == 0 {
		return // ignore
	}
	ct := t.RespHeader.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") &&
		!strings.HasPrefix(ct, "application/xml") {
		t.RespBody = []byte("###response content type: " + ct)
		return // ignore
	}
	if err != nil {
		t.RespBody = []byte("###error copy body, " + err.Error())
	} else if bsz <= 0 {
		// body is empty
	} else if int(bsz) < cap(buf) {
		t.RespBody = make([]byte, bsz)
		copy(t.RespBody, buf[:bsz])
	} else {
		// 缓存区的内容，可以通过 ReverseProxy.BufferPool.defCap 调整缓存区大小，
		// 默认 64K， 取自 Linux 系统 UDP 缓存区大小。
		t.RespBody = []byte("###response body too large, skip")
	}
}

func (t *Record0) SetRespBody(str string) {
	t.RespBody = []byte(str)
}

// 追踪记录, 将日志写入日志系统中
func (rt *Record0) _track() {
	if rt._abort {
		return // ignore
	}
	rt.ServeTime = time.Now().UnixMilli() - rt.StartTime
	rt._abort = true
	if rt.Save != nil {
		rt.Save(rt)
	}
	rt.Cleanup()
}

func (t *Record0) _recycle() {
	t._track()
	if t.Pool != nil {
		t.Pool.Put(t)
	}
}

func (t *Record0) Recycle() {
	go t._recycle() // 异步处理
}
