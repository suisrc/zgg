package gtw

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/suisrc/zgg/z"
)

var (
	GenStr        = z.GenStr
	GenUUIDv4     = z.GenUUIDv4
	GetRemoteIP   = z.GetRemoteIP
	NewBufferPool = z.NewBufferPool
)

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

func NewTargetProxy3(target_ string) (*GatewayProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &GatewayProxy{ReverseProxy: ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
		},
		BufferPool: NewBufferPool(0, 0),
	}}, nil
}

func NewDomainProxy3(target_, domain string) (*GatewayProxy, error) {
	target, err := url.Parse(target_)
	if err != nil {
		return nil, err
	}
	return &GatewayProxy{ReverseProxy: ReverseProxy{
		Director: func(req *http.Request) {
			RewriteRequestURL(req, target)
			req.Host = domain
		},
		BufferPool: NewBufferPool(0, 0),
	}}, nil
}
