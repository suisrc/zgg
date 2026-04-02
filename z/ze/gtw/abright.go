// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package gtw

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
)

/*

// 代理规则：
// /~/  开头的，使用 a 地址完全取代; /xx/zz/... -> /~/vv = /vv/...
// /-/  开头的，使用 a 地址截取 b 地址; /xx/zz/... -> /-/xx = /zz/...，这里是尝试截取，如果不存在，会忽略掉
// /... 其他的，合并地址; /xx/zz/... -> /vv = /vv/xx/zz/...

*/

var (
	ErrNil = errors.New("<nil>") // 处理业务过程中，用于跳过错误

	GenStr        = z.GenStr
	GenUUIDv4     = z.GenUUIDv4
	GetRemoteIP   = z.GetRemoteIP
	NewBufferPool = z.NewBufferPool
	GetAction     = z.GetAction

	// NewTargetProxy0(原版，httputil) or NewTargetProxy2(扩展，支持 /~/ 和 /-/ 格式)
	NewTargetProxy func(target string) (http.Handler, error) = NewTargetProxy2
	// NewDomainProxy0(原版，httputil) or NewDomainProxy2(扩展，支持 /~/ 和 /-/ 格式)
	NewDomainProxy func(target, domain string) (http.Handler, error) = NewDomainProxy2
)

func NewTargetProxy0(target_ string) (http.Handler, error) {
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

func NewDomainProxy0(target_, domain string) (http.Handler, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	if domain == "" {
		domain = target.Host
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
	TransportDefault = http.DefaultTransport

	// default's transport for gtw
	TransportGtw0 = &http.Transport{
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
			InsecureSkipVerify: false,
		},
	}

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

// --------------------------------------------------------------------------------------

func NewTargetProxy2(target_ string) (http.Handler, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL2(req, target)
		},
		BufferPool: NewBufferPool(0, 0),
		// Transport: TransportSkip,
	}, nil
}

func NewDomainProxy2(target_, domain string) (http.Handler, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	if domain == "" {
		domain = target.Host
	}
	return &ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL2(req, target)
			req.Host = domain
		},
		BufferPool: NewBufferPool(0, 0),
		// Transport: TransportSkip,
	}, nil
}

func NewTargetGateway2(target_ string, pool BufferPool) (*GatewayProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		pool = NewBufferPool(0, 0)
	}
	return &GatewayProxy{ReverseProxy: ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL2(req, target)
		},
		BufferPool: pool,
	}}, nil
}

func NewDomainGateway2(target_, domain string, pool BufferPool) (*GatewayProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	if domain == "" {
		domain = target.Host
	}
	if pool == nil {
		pool = NewBufferPool(0, 0)
	}
	return &GatewayProxy{ReverseProxy: ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL2(req, target)
			req.Host = domain
		},
		BufferPool: pool,
	}}, nil
}

//---------------------------------------------------------------------------------------------
// reverse 扩展

func RewriteRequestURL2(req *http.Request, target *url.URL) {
	targetQuery := target.RawQuery
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path, req.URL.RawPath = joinURLPath2(target, req.URL)
	if targetQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}
	if z.IsDebug() {
		z.Logn("[rewrite2]: ->", req.URL.String())
	}
}

// /~/  开头的，使用 a 地址完全取代; /xx/zz/... -> /~/vv = /vv/...
// /-/  开头的，使用 a 地址截取 b 地址; /xx/zz/... -> /-/xx = /zz/...，这里是尝试截取，如果不存在，会忽略掉
// /... 其他的，合并地址; /xx/zz/... -> /vv = /vv/xx/zz/...
func joinURLPath2(a, b *url.URL) (path, rawpath string) {
	// z.Logn("[_gateway]: ===========", a.Path, a.RawPath)
	if strings.HasPrefix(a.Path, "/~/") {
		if a.RawPath == "" {
			return a.Path[2:], ""
		}
		return a.Path[2:], a.RawPath[2:]
	}
	if strings.HasPrefix(a.Path, "/-/") {
		if a.RawPath == "" {
			return strings.TrimPrefix(b.Path, a.Path[2:]), ""
		}
		return strings.TrimPrefix(b.Path, a.Path[2:]), //
			strings.TrimPrefix(b.RawPath, a.RawPath[2:])
	}
	// 当 URL 路径仅包含合法字符（字母、数字、-、_、.、~）时，RawPath 会为空
	if a.RawPath == "" && b.RawPath == "" {
		aslash := strings.HasSuffix(a.Path, "/")
		bslash := strings.HasPrefix(b.Path, "/")
		switch {
		case aslash && bslash:
			return a.Path + b.Path[1:], ""
		case !aslash && !bslash:
			return a.Path + "/" + b.Path, ""
		}
		return a.Path + b.Path, ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}

//---------------------------------------------------------------------------------------------
// reverse 原版

// NewSingleProxy returns a new [ReverseProxy] that routes
// URLs to the scheme, host, and base path provided in target. If the
// target's path is "/base" and the incoming request was for "/dir",
// the target request will be for /base/dir.
//
// NewSingleProxy does not rewrite the Host header.
//
// To customize the ReverseProxy behavior beyond what
// NewSingleProxy provides, use ReverseProxy directly
// with a Rewrite function. The ProxyRequest SetURL method
// may be used to route the outbound request. (Note that SetURL,
// unlike NewSingleProxy, rewrites the Host header
// of the outbound request by default.)
//
//	proxy := &ReverseProxy{
//		Rewrite: func(r *ProxyRequest) {
//			RewriteRequestURL(r.Out, target)
//			// r.Out.Host = ""
//			r.Out.Host = r.In.Host // if desired
//		},
//	}
func NewSingleProxy(target *url.URL) *ReverseProxy {
	director := func(req *http.Request) {
		RewriteRequestURL(req, target)
	}
	return &ReverseProxy{Director: director}
}

func RewriteRequestURL(req *http.Request, target *url.URL) {
	targetQuery := target.RawQuery
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
	if targetQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}
