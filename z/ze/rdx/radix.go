// Copyright 2013 Julien Schmidt. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/julienschmidt/httprouter/blob/master/LICENSE.

package rdx

import (
	"net/http"

	"github.com/suisrc/zgg/z"
)

// -----------------------------------------------------------------------------------
// 基于 Tire / Radix Tree 的 httprouter 的路由
// 也是为外部扩展路由提供标准实现方式
// 路由算法引用 https://github.com/julienschmidt/httprouter

func init() {
	z.Engines["rdx"] = NewRdxRouter
}

// var _ z.Engine = (*RdxRouter)(nil)

func NewRdxRouter(svckit z.SvcKit) z.Engine {
	return &RdxRouter{
		name:   "zgg-rdx",
		svckit: svckit,
		Router: New(),
	}
}

type RdxRouter struct {
	name   string
	svckit z.SvcKit
	Router *Router
}

func (aa *RdxRouter) Name() string {
	return aa.name
}

func (aa *RdxRouter) Handle(method, action string, handle z.HandleFunc) {
	path := "/" + action
	if method == "" {
		method = "GET" // 默认使用 GET
	}
	aa.Router.Handle(method, path, func(rw http.ResponseWriter, rr *http.Request, ps Params) {
		ctx := z.NewCtx(aa.svckit, rr, rw, aa.name)
		ctx.Params = ps.ByName
		defer ctx.Cancel()
		handle(ctx)
	})
}

func (aa *RdxRouter) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	aa.Router.ServeHTTP(rw, rr)
}
