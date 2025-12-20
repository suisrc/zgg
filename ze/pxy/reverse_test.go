// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pxy_test

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"

	"github.com/suisrc/zgg/ze/pxy"
)

var (
	transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
)

func Test_proxy(t *testing.T) {
	// 可以适当的增加一些令牌信息等内容
	target, _ := url.Parse("http://127.0.0.1:8080")
	proxy := pxy.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(nil, nil) // next
}

func Test_rewrite(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:8080")
	domain := "www.exp.com"
	director := func(req *http.Request) { pxy.RewriteRequestDomain(req, target, domain) }
	proxy := &pxy.ReverseProxy{Director: director}
	proxy.Transport = transport
	proxy.ServeHTTP(nil, nil)
}
