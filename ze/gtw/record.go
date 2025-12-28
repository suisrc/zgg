package gtw

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
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
	UpstreamTime int64       // upstream time

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

// -------------------------------------------------------------------

var (
	loc_areaip = ""
	host_name_ = ""
	namespace_ = ""
	serve_name = ""
)

// 获取局域网地址
func GetLocAreaIp() string {
	if loc_areaip != "" {
		return loc_areaip
	}
	hnam := GetHostname()
	if hnam == "" || hnam == "localhost" {
		loc_areaip = "127.0.0.1"
		return loc_areaip // 无法解析当前节点的局域网地址
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
			if len(ips) < 2 || ips[0] == "127.0.0.1" {
				continue
			}
			found := false
			for _, name := range ips[1:] {
				if EqualFold(name, hnam) {
					found = true
					break // 找到与 hostname 匹配的 IP
				}
			}
			if !found {
				continue
			}
			// 判断是否为 IPv4 地址
			if ip := net.ParseIP(strings.TrimSpace(ips[0])); ip == nil {
			} else if v4 := ip.To4(); v4 == nil {
			} else if v4.IsLoopback() {
			} else {
				loc_areaip = strings.TrimSpace(ips[0])
				break
			}
		}
	}
	if loc_areaip == "" {
		loc_areaip = "127.0.0.1" // 无法解析
	}
	return loc_areaip
}

// 获取局域网域名
func GetHostname() string {
	if host_name_ != "" {
		return host_name_
	}
	host_name_, _ = os.Hostname()
	if host_name_ == "" {
		host_name_ = "localhost"
	}
	return host_name_
}

func GetNamespace() string {
	if namespace_ != "" {
		return namespace_
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		z.Printf("unable to read namespace: %s, return '-'", err.Error())
		namespace_ = "-"
	} else {
		namespace_ = string(ns)
	}
	return namespace_
}

func GetServeName() string {
	if serve_name != "" {
		return serve_name
	}
	ns := GetNamespace()
	if ns == "-" {
		serve_name = "serv." + GetHostname() // 不是 k8s
	} else {
		serve_name = "kube." + ns + "." + GetHostname() // k8s 环境
	}
	return serve_name
}
