package gtw

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

func NewTargetProxy2(target_ string) (*ReverseProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
		},
		BufferPool: NewBufferPool(0, 0),
	}, nil
}

func NewDomainProxy2(target_, domain string) (*ReverseProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
			req.Host = domain
		},
		BufferPool: NewBufferPool(0, 0),
	}, nil
}

func NewTargetProxy(target_ string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &httputil.ReverseProxy{
		// Rewrite: func(req *httputil.ProxyRequest) {
		// 	req.SetURL(target)
		// 	req.Out.Host = req.In.Host
		// },
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
		},
		BufferPool: NewBufferPool(0, 0),
		// Transport:  TransportSkip,
	}, nil
}

func NewDomainProxy(target_, domain string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &httputil.ReverseProxy{
		// Rewrite: func(req *httputil.ProxyRequest) {
		// 	req.SetURL(target)
		// 	req.Out.Host = domain
		// },
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
			req.Host = domain
		},
		BufferPool: NewBufferPool(0, 0),
		// Transport: TransportSkip,
	}, nil
}

// --------------------------------------------------------------------------------------

var (
	// default's transport
	TransportDeft = http.DefaultTransport

	// skip tls verify's transport
	TransportSkip = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
)

// NewBufferPool 初始化缓冲池
// defCap: 新缓冲区的默认容量（如4KB）
// maxCap: 允许归还的最大容量（如1MB）
func NewBufferPool(defCap, maxCap int) BufferPool {
	if defCap <= 0 {
		defCap = 32 * 1024 // 默认4KB
	}
	if maxCap <= 0 {
		maxCap = 1024 * 1024 // 默认1MB
	}
	return &BufferPool0{
		defCap: defCap,
		maxCap: maxCap,
		pool: &sync.Pool{
			New: func() any {
				// 创建默认容量的空字节切片（len=0，cap=defaultCap）
				return make([]byte, 0, defCap)
			},
		},
	}
}

// BufferPool0 字节缓冲池：基于sync.Pool实现
type BufferPool0 struct {
	pool   *sync.Pool
	maxCap int // 允许归还的最大缓冲区容量（避免超大缓冲区占用内存）
	defCap int // 新创建缓冲区的默认容量
}

// Get 获取缓冲区：从池取出或创建新缓冲区
func (p *BufferPool0) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put 归还缓冲区：重置后放回池（若容量超过maxCap则丢弃）
func (p *BufferPool0) Put(buf []byte) {
	// 1. 检查缓冲区容量是否超过限制
	if cap(buf) > p.maxCap {
		return
	}
	// 2. 重置缓冲区：保留容量，清空内容（len=0）
	p.pool.Put(buf[:0])
}
