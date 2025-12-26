package gtw

import (
	"net/http"
	"sync"
)

type RecordTrace interface {
	LogRequest(req *http.Request)
	LogOutRequest(out *http.Request)
	LogResponse(res *http.Response)
	LogRespBody(bsz int64, err error, buf []byte)
	SetRespBody(string)
	Recycle()
	Cleanup() RecordTrace
	SetUpstream(addr string)
	SetSrvAuthz(addr string)
}

// 日志处理句柄
type RecordFunc func(record RecordTrace)

// 记录内容追踪
type RecordPool interface {
	Get() RecordTrace
	Put(RecordTrace)
}

// --------------------------------------------------------------------

var _ RecordTrace = (*Record0)(nil)

// 日志内容追踪
type Record0 struct {
	Pool RecordPool `json:"-"` // 缓冲池
	Save RecordFunc `json:"-"` // 处理者

	TraceID   string // trace id
	RemoteIP  string // remote ip
	UserAgent string // user agent
	Referer   string // page info
	ClientID  string // client id

	Scheme     string      // request scheme
	Method     string      // request method
	ReqHost    string      // request origin host
	ReqURL     string      // request origin url
	ReqHeader  http.Header // request origin header
	ReqBody    []byte      // request body
	RemoteAddr string      // remote address

	OutReqHost   string      // request header
	OutReqURL    string      // request url
	OutReqHeader http.Header // request header
	UpstreamAddr string      // upstream address
	SrvAuthzAddr string      // serve authz address

	RespHeader http.Header // response header
	RespBody   []byte      // response body
	RespSize   int64       // response body size
	StatusCode int         // status code

	Expand map[string]any          // 扩展字段
	Cookie map[string]*http.Cookie // cookie

	StartTime int64 // 开始时间, 毫秒
	ServeTime int64 // 服务时间, 毫秒, 请求处理时间
	_abort    bool  // 是否终止
}

func (rt *Record0) Cleanup() RecordTrace {
	rt._abort = false

	rt.TraceID = ""
	rt.ClientID = ""
	rt.RemoteIP = ""
	rt.UserAgent = ""
	rt.Referer = ""

	rt.Scheme = ""
	rt.Method = ""
	rt.ReqHost = ""
	rt.ReqURL = ""
	rt.ReqHeader = nil
	rt.ReqBody = nil
	rt.RemoteAddr = ""

	rt.OutReqHost = ""
	rt.OutReqURL = ""
	rt.OutReqHeader = nil
	rt.UpstreamAddr = ""

	rt.RespHeader = nil
	rt.RespBody = nil
	rt.RespSize = 0
	rt.StatusCode = 0

	// Expand 内容少， delete 比 make 实际场景性能高
	for k := range rt.Expand {
		delete(rt.Expand, k)
	}
	for k := range rt.Cookie {
		delete(rt.Cookie, k)
	}

	rt.StartTime = 0
	rt.ServeTime = 0

	return rt
}

func (rc *Record0) SetUpstream(addr string) {
	rc.UpstreamAddr = addr
}

func (rc *Record0) SetSrvAuthz(addr string) {
	rc.SrvAuthzAddr = addr
}

// ----------------------------------------------------------------------------

// NewRecordPool 初始化缓冲池
func NewRecordPool(save RecordFunc) RecordPool {
	pool := &RecordPool0{
		pool: &sync.Pool{},
		save: save,
	}
	pool.pool.New = func() any {
		return &Record0{
			Pool:   pool,
			Save:   save,
			Expand: make(map[string]any),
			Cookie: make(map[string]*http.Cookie),
		}
	}
	return pool

}

// RecordPool0 记录内容复用池
type RecordPool0 struct {
	pool *sync.Pool
	save RecordFunc
}

// Get
func (p *RecordPool0) Get() RecordTrace {
	return p.pool.Get().(RecordTrace)
}

// Put
func (p *RecordPool0) Put(rt RecordTrace) {
	p.pool.Put(rt.Cleanup())
}
