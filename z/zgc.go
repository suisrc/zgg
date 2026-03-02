// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// zgc: z? golang context

package z

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sync"
	"text/template"
)

// 定义处理函数
type HandleFunc func(rc *Ctx)

// map any
type HA map[string]any

// map str
type HM map[string]string

// get str
type GetVal func(key string) string

// 请求上下文内容
type Ctx struct {
	Ctx context.Context
	// Cancel func
	Cancel context.CancelFunc
	// All Module
	SvcKit SvcKit
	// Request action
	Action string
	// Request Cache
	Caches HA
	// Params
	Params GetVal
	// Request
	Request *http.Request
	// Response
	Writer http.ResponseWriter
	// Trace ID
	TraceID string
	// X-Request-Rt
	ReqType string
	// flag router name
	_router string
	// flag action abort
	_abort bool
}

// 用于标记提前结束，不是强制的
func (ctx *Ctx) Abort() {
	ctx._abort = true
}

func (ctx *Ctx) IsAbort() bool {
	return ctx._abort
}

// 已 JSON 格式写出响应
func (ctx *Ctx) JSON(err error) {
	ctx._abort = true // rc.Abort()
	// 注意，推荐使用 JSON(rc, rs), 这里只是为了简化效用逻辑
	switch err := err.(type) {
	case *Result:
		JSON(ctx, err)
	default:
		JSON(ctx, &Result{ErrCode: "unknow-error", Message: err.Error()})
	}
}

// 已 HTML 模板格式写出响应
func (ctx *Ctx) HTML(tpl string, res any, hss int) {
	ctx._abort = true
	if ctx.TraceID != "" {
		ctx.Writer.Header().Set("X-Request-Id", ctx.TraceID)
	}
	if hss > 0 {
		ctx.Writer.WriteHeader(hss)
	}
	HTML0(ctx.SvcKit.Zgg(), ctx.Request, ctx.Writer, res, tpl)
}

// 已 TEXT 模板格式写出响应
func (ctx *Ctx) TEXT(txt string, hss int) {
	ctx._abort = true
	if ctx.TraceID != "" {
		ctx.Writer.Header().Set("X-Request-Id", ctx.TraceID)
	}
	ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if hss > 0 {
		ctx.Writer.WriteHeader(hss) // 最后写状态码头
	}
	ctx.Writer.Write([]byte(txt))
}

// 已 BYTE 模板格式写出响应
func (ctx *Ctx) BYTE(bts io.Reader, hss int, cty string) {
	ctx._abort = true
	if ctx.TraceID != "" {
		ctx.Writer.Header().Set("X-Request-Id", ctx.TraceID)
	}
	if cty != "" {
		ctx.Writer.Header().Set("Content-Type", cty)
	}
	if hss > 0 {
		ctx.Writer.WriteHeader(hss) // 最后写状态码头
	}
	io.Copy(ctx.Writer, bts)
}

// 已 JSON 错误格式写出响应
func (ctx *Ctx) JERR(err error, hss int) {
	ctx._abort = true // rc.Abort()
	// 注意，推荐使用 JSON(rc, rs), 这里只是为了简化效用逻辑
	var res *Result
	switch err := err.(type) {
	case *Result:
		res = err
	default:
		res = &Result{ErrCode: "unknow-error", Message: err.Error()}
	}
	if hss > 0 {
		res.Status = hss
	}
	JSON(ctx, res)
}

// 创建上下文函数
func NewCtx(svckit SvcKit, request *http.Request, writer http.ResponseWriter, router string) *Ctx {
	action := GetAction(request.URL)
	ctx := &Ctx{SvcKit: svckit, Action: action, Caches: HA{}, Request: request, Writer: writer}
	ctx._router = router
	ctx.Ctx, ctx.Cancel = context.WithCancel(context.Background())
	ctx.TraceID = GetTraceID(request)
	ctx.ReqType = GetReqType(request)
	return ctx
}

// 获取请求 action
// 1. 优先使用 query.action
// 2. 其次使用 path[1:] 作为 action, 注意，如果需要补全path， 需要增加 /
func GetAction(uu *url.URL) string {
	action := uu.Query().Get("action")
	if action == "" {
		rpath := uu.Path
		if len(rpath) > 0 {
			rpath = rpath[1:] // 删除前缀 '/'
		}
		action = rpath
	}
	return action
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

// 定义响应结构体
type Result struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	ErrCode string `json:"errcode,omitempty"`
	Message string `json:"message,omitempty"`
	ErrShow int    `json:"errshow,omitempty"`
	TraceID string `json:"traceid,omitempty"`
	Total   *int   `json:"total,omitempty"`

	Ctx    *Ctx   `json:"-"`
	Status int    `json:"-"`
	Header HM     `json:"-"`
	TplKey string `json:"-"`
}

func (aa *Result) Error() string {
	return fmt.Sprintf("[%v], %s, %s", aa.Success, aa.ErrCode, aa.Message)
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

// 响应 JSON 结果, 这是一个套娃，
func JSON(ctx *Ctx, res *Result) {
	res.Ctx = ctx
	// TraceID 可能不存在，如果不是 '' 则 PASS
	if res.TraceID == "" {
		res.TraceID = ctx.TraceID
	}
	if res.TraceID != "" {
		ctx.Writer.Header().Set("X-Request-Id", res.TraceID)
	}
	if !res.Success && res.ErrShow <= 0 {
		res.ErrShow = 1
	}
	// 响应其他头部
	if ctx.Request.Header != nil { // 设置响应头
		for k, v := range res.Header {
			ctx.Writer.Header().Set(k, v)
		}
	}
	// 响应结果
	switch ctx.ReqType {
	case "2":
		JSON2(ctx.Request, ctx.Writer, res)
	case "3":
		HTML3(ctx.Request, ctx.Writer, res)
	default:
		JSON0(ctx.Request, ctx.Writer, res)
	}
}

// 响应 JSON 结果: content-type http-status json-data
func JSON0(rr *http.Request, rw http.ResponseWriter, rs *Result) {
	// 响应结果
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	if rs.Status > 0 {
		rw.WriteHeader(rs.Status)
	}
	json.NewEncoder(rw).Encode(rs)
}

// 以 '2' 形式格式化, 响应 JSON 结果: content-type http-status json-data
func JSON2(rr *http.Request, rw http.ResponseWriter, rs *Result) {
	// 转换结构
	ha := HA{"success": rs.Success}
	if rs.Data != nil {
		ha["data"] = rs.Data
	}
	if rs.ErrCode != "" {
		ha["errorCode"] = rs.ErrCode
		ha["errorMessage"] = rs.Message
	}
	if rs.ErrShow > 0 {
		ha["showType"] = rs.ErrShow
	}
	if rs.TraceID != "" {
		ha["traceId"] = rs.TraceID
	}
	if rs.Total != nil {
		ha["total"] = rs.Total
	}
	// 响应结果
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	if rs.Status > 0 {
		rw.WriteHeader(rs.Status)
	}
	json.NewEncoder(rw).Encode(ha)
}

// 以 '3' 形式格式化, 选择模版，响应 HTML 模板结果: content-type http-status html-data
func HTML3(rr *http.Request, rw http.ResponseWriter, rs *Result) {
	if rs.Ctx == nil {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.Write([]byte("template render error: request content not found"))
		return
	}
	tmpl := rs.TplKey
	if tmpl == "" {
		if rs.Success {
			tmpl = "success.html"
		} else {
			tmpl = "error.html"
		}
	}
	if rs.Status > 0 {
		rw.WriteHeader(rs.Status)
	}
	HTML0(rs.Ctx.SvcKit.Zgg(), rr, rw, rs, tmpl)
}

// 响应 HTML 模板结果: content-type http-status html-data
func HTML0(zg *Zgg, rr *http.Request, rw http.ResponseWriter, rs any, tp string) {
	// 响应结果
	if zg == nil {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.Write([]byte("template render error: server not found"))
	} else {
		err := zg.TplKit.Render(rw, tp, rs)
		if err != nil {
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			rw.Write([]byte("template render error: " + err.Error()))
		} else {
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
	}
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

// 在 init 注册配置函数
func Register(key string, opt OptionFunc) {
	optlock.Lock() // 注册方法，全局锁即可
	defer optlock.Unlock()
	// options = append(options, Ref[string, OptionFunc]{Key: key, Val: opt})
	idx := slices.IndexFunc(options, func(opt Ref[string, OptionFunc]) bool {
		return opt.Key > key
	})
	ref := Ref[string, OptionFunc]{Key: key, Val: opt}
	if idx < 0 {
		options = append(options, ref)
	} else {
		options = slices.Insert(options, idx, ref)
	}

	// debug print console
	// Print("options: |")
	// for _, opt := range options[:] {
	// 	Printf(" %v |", opt.Key)
	// }
	// Println()
}

// GET http method
func GET(action string, hdl HandleFunc, zgg *Zgg) {
	zgg.AddRouter(http.MethodGet+" "+action, hdl)
}

// POST http method
func POST(action string, hdl HandleFunc, zgg *Zgg) {
	zgg.AddRouter(http.MethodPost+" "+action, hdl)
}

/**
 * 注册服务, key 必须唯一, 如果 key 为空， 使用 val.(type).Name() 作为 key
 * @param kit 服务容器
 * @param inj 自动注入
 * @param key 服务 key
 * @param val 服务实例
 */
func RegKey[T any](kit SvcKit, inj bool, key string, val T) T {
	if key == "" {
		key = reflect.TypeOf(val).Elem().Name()
	}
	kit.Set(key, val)
	if inj {
		kit.Inj(val) // 自动注入， 可以注入自己
	}
	return val
}

// 注册服务， name = val.(type)
func RegSvc[T any](kit SvcKit, val T) T {
	key := reflect.TypeOf(val).Elem().Name()
	kit.Set(key, val)
	return val
}

// 注册服务， name = val.(type)， 并自动初始化 val 实体
func Inject[T any](kit SvcKit, val T) T {
	key := reflect.TypeOf(val).Elem().Name()
	kit.Set(key, val).Inj(val) // 自动注入， 可以注入自己
	return val
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

// close function
type Closed func()

// 定义配置函数
type OptionFunc func(*Zgg) Closed

var (
	// 应用配置列表，依据 key 排序，初始化顺序
	options = []Ref[string, OptionFunc]{}

	optlock = sync.Mutex{} // 注册方法，全局锁即可
	// optlock = sync.RWMutex{}
	// optlock = xsync.NewRBMutex()
)

type Tpl struct {
	Key string             // 模版编码
	Tpl *template.Template // 模版
	Err error              // 加载模版的异常
	Txt string             // 模版原始内容
	Lck sync.Mutex         // 模版锁
}

// 模版工具接口
type TplKit interface {
	Get(key string) *Tpl
	Render(wr io.Writer, name string, data any) error
	Load(key string, str string) *Tpl
	Preload(dir string) error
}

// 服务工具接口
type SvcKit interface {
	Zgg() *Zgg                      // 获取模块管理器 *Zgg 接口
	Get(key string) any             // 获取服务
	Set(key string, val any) SvcKit // 增加服务 val = nil 是卸载服务
	Map() map[string]any            // 服务列表, 注意，是副本
	Inj(obj any) SvcKit             // 注册服务 injec 使用 `svckit:"xxx"` 初始化服务
}

// 引擎接口, Engine, 不适用 Router 是为了和 其他 Router 名字上区分开。以便于支持多 Router 而不会出现冲突
type Engine interface {
	Name() string                                       // router engine name
	Handle(method, action string, handle HandleFunc)    // register router handle, [method]可能为"", [action]开头无"/"
	ServeHTTP(rw http.ResponseWriter, rr *http.Request) // http.HandlerFunc
}

type EngineBuilder func(SvcKit) Engine
