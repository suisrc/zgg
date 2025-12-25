package gtw

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
)

// 日志处理句柄
type RecordHandler interface {
	Handle(record *RecordTrace)
}

// 日志内容追踪
type RecordTrace struct {
	Pool    RecordPool    `json:"-"` // 缓冲池
	Handler RecordHandler `json:"-"` // 处理者

	TraceID   string // trace id
	ClientID  string // client id
	RemoteIP  string // remote ip
	Referer   string // page info
	UserAgent string // user agent

	Method    string      // request method
	ReqHost   string      // request origin host
	ReqURL    string      // request origin url
	ReqHeader http.Header // request origin header
	ReqBody   []byte      // request body

	OutReqHost   string      // request header
	OutReqURL    string      // request url
	OutReqHeader http.Header // request header

	RespHeader http.Header // response header
	RespBody   []byte      // response body
	RespSize   int64       // response body size
	StatusCode int         // status code
	_abort     bool        // 是否终止

	Expand map[string]any // 扩展字段
}

// 记录原始请求内容
func (t *RecordTrace) LogRequest(req *http.Request) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	// trace id
	t.TraceID = req.Header.Get("X-Request-Id")
	t.UserAgent = req.UserAgent()
	t.Referer = req.Referer()

	t.Method = req.Method
	t.ReqHost = req.Host
	t.ReqURL = req.URL.String()
	t.ReqHeader = req.Header

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
func (t *RecordTrace) LogOutRequest(outreq *http.Request) {
	t.OutReqHost = outreq.Host
	t.OutReqURL = outreq.URL.String()
	t.OutReqHeader = outreq.Header // record request header
}

// 记录请求结果
func (t *RecordTrace) LogResponse(res *http.Response) {
	t.RespHeader = res.Header
	t.StatusCode = res.StatusCode
	if res.StatusCode == http.StatusSwitchingProtocols {
		t.RespBody = []byte("###response body is websocket, skip")
		t._track() // 提前记录 websocket 请求内容
	}
}

// 记录请求body内容
func (t *RecordTrace) LogRespBody(bsz int64, err error, buf []byte) {
	if t.RespHeader == nil {
		return // ignore
	}
	ct := t.ReqHeader.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") &&
		!strings.HasPrefix(ct, "application/xml") {
		t.RespBody = []byte("###response content type: " + ct)
		return // ignore
	}
	t.RespSize = bsz
	if err != nil {
		t.RespBody = []byte("###error copy body, " + err.Error())
	} else if bsz <= 0 {
		// body is empty
	} else if int(bsz) < cap(buf) {
		t.RespBody = bytes.Clone(buf[:bsz]) // 拷贝
	} else {
		// 缓存区的内容，可以通过 ReverseProxy.BufferPool.defCap 调整缓存区大小，
		// 默认 64K， 取自 Linux 系统 UDP 缓存区大小。
		t.RespBody = []byte("###response body too large, skip")
	}
}

func (t *RecordTrace) Recycle() {
	t._track()
	if t.Pool != nil {
		t.Pool.Put(t)
	}
}

// 追踪记录, 将日志写入日志系统中
func (rt *RecordTrace) _track() {
	if rt._abort {
		return // ignore
	}
	rt._abort = true
	if rt.Handler != nil {
		rt.Handler.Handle(rt)
	}
	// t.Handler = nil
	rt.Reset()
}

func (rt *RecordTrace) Reset() *RecordTrace {
	rt._abort = false
	rt.Method = ""
	rt.ReqHost = ""
	rt.ReqURL = ""
	rt.ReqHeader = nil
	rt.ReqBody = nil

	rt.OutReqHost = ""
	rt.OutReqURL = ""
	rt.OutReqHeader = nil

	rt.RespHeader = nil
	rt.RespBody = nil
	rt.RespSize = 0
	rt.StatusCode = 0
	return rt
}

// ----------------------------------------------------------------------------

type RecordPool interface {
	Get() *RecordTrace
	Put(*RecordTrace)
}

// NewRecordPool 初始化缓冲池
func NewRecordPool(handler RecordHandler) RecordPool {
	return &RecordPool0{
		handler: handler,
		pool: &sync.Pool{
			New: func() any {
				return &RecordTrace{}
			},
		},
	}
}

// RecordPool0 记录内容复用池
type RecordPool0 struct {
	handler RecordHandler
	pool    *sync.Pool
}

// Get
func (p *RecordPool0) Get() *RecordTrace {
	rt := p.pool.Get().(*RecordTrace)
	rt.Pool = p
	rt.Handler = p.handler
	return rt
}

// Put
func (p *RecordPool0) Put(rt *RecordTrace) {
	rt.Pool = nil
	rt.Handler = nil
	p.pool.Put(rt.Reset())
}

// ----------------------------------------------------------------------------
