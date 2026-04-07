// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// zgg(z? golang google) 核心内容，为简约而生

package z

import (
	"cmp"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
	"unsafe"
)

var (
	AppName = "zgg"
	Version = "v0.0.0"
	AppInfo = "(https://github.com/suisrc/zgg.git)"

	HttpServeDef = true // 启动默认 http 服务?

	// 路由构建器
	Engines = map[string]EngineBuilder{
		"map": NewMapRouter,
		"mux": NewMuxRouter,
	}

	C = new(struct {
		Server ServerConfig
	})

	ResultEncoders = map[string]ResultEncoder{
		"2": EncodeJson2,
		"3": EncodeHtml3,
	}

	IngoreErr = errors.New("ignore error")
)

func PrintVersion() {
	Logn(AppName, Version, AppInfo)
}

// 默认配置， Server配置需要内嵌该结构体
type ServerConfig struct {
	Fxser   bool   `json:"xser"` // 标记 xser 头部信息
	Local   bool   `json:"local"`
	Addr    string `json:"addr" default:"0.0.0.0"`
	Port    int    `json:"port" default:"80"`
	Ptls    int    `json:"ptls" default:"443"`
	Dual    bool   `json:"dual"`   // http and https
	Engine  string `json:"engine"` // router engine
	ApiPath string `json:"api"`    // root api path
	TplPath string `json:"tpl"`    // templates folder path
	ReqXrtd string `json:"xrt"`    // X-Request-Rt default value, 1: zgg, 2: ali, 3: html
}

// -----------------------------------------------------------------------------------

var _ http.Handler = (*Zgg)(nil)

// 默认服务实体
type Zgg struct {
	Servers map[string]Server
	Closeds []Closed    // 模块关闭函数列表
	TLSConf *tls.Config // Certificates, GetCertificate

	Engine Engine // 路由引擎
	SvcKit SvcKit // 服务工具
	TplKit TplKit // 模版工具
	_abort bool   // 终止标记
}

// -----------------------------------------------------------------------------------

// 服务初始化
func (aa *Zgg) ServeInit() bool {
	aa.Servers = map[string]Server{}
	aa.Closeds = []Closed{}
	if aa.SvcKit == nil {
		aa.SvcKit = NewSvcKit(aa)
	}
	if builder, ok := Engines[C.Server.Engine]; !ok {
		Logf("[_router_]: router not found by [-eng %s]\n", C.Server.Engine)
		return false
	} else {
		aa.Engine = builder(aa.SvcKit)
		Logf("[_router_]: build %s.router by [-eng %s]\n", aa.Engine.Name(), C.Server.Engine)
	}
	if aa.TplKit == nil {
		aa.TplKit = NewTplKit(aa)
		if C.Server.TplPath != "" {
			err := aa.TplKit.Preload(C.Server.TplPath)
			if err != nil {
				Logf("[_tplkit_]: Preload error: %v\n", err)
			}
		}
	}
	// -----------------------------------------------
	Logn("[register]: register server options...")
	for _, opt := range options {
		if opt.Val == nil {
			continue
		}
		if IsDebug() {
			ekey := opt.Key
			if size := len(ekey); size < 72 {
				ekey += " " + strings.Repeat("-", 71-size)
			}
			Logf("[register]: %s", ekey)
		}
		cls := opt.Val(aa)
		if cls != nil {
			aa.Closeds = append(aa.Closeds, cls)
		}
		if aa._abort {
			Logn("[register]: serve already stop! exit...")
			return false // 退出
		}
	}
	slices.Reverse(aa.Closeds) // 倒序, 后进先出
	return true
}

// 服务终止，注意，这里只会终止模版，不会终止服务， 终止服务，需要调用 hsv.Shutdown
func (aa *Zgg) ServeStop(err ...string) {
	if len(err) > 0 {
		Logz(1, "[_server_]: serve stop,", strings.Join(err, " "))
	}
	if aa._abort {
		return
	}
	aa._abort = true
	if aa.Closeds != nil {
		for _, cls := range aa.Closeds {
			cls() // 模块关闭
		}
	}
	Logn("[_server_]: services have been terminated")
}

// 启动 HTTP 服务
func (aa *Zgg) RunServe() {
	// 停止业务模块， 先停服务，后停模块
	defer aa.ServeStop()
	// 启动HTTP服务， 并可优雅的终止
	for key, srv := range aa.Servers {
		if srv != nil {
			go srv.RunServe(key)
		}
	}
	// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
	aa.WaitFor()
}

// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
func (aa *Zgg) WaitFor() {
	if len(aa.Servers) == 0 {
		Logn("[_server_]: no server to wait for, exit...")
		return
	}
	ssc := make(chan os.Signal, 1)
	signal.Notify(ssc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ssc
	Logn("[_server_]: services is shutting down...")
	// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for key, srv := range aa.Servers {
		if srv != nil {
			Logn("[_server_]: http server stoping...", key)
			if err := srv.Shutdown(ctx); err != nil {
				Logn("[_server_]: http server shutdown error:", key, err)
			}
		}
	}
}

type Server interface {
	RunServe(key string)
	Shutdown(ctx context.Context) error
}

func NewServer(handler http.Handler, addr string, conf *tls.Config) Server {
	return &servez{Server: http.Server{Handler: handler, Addr: addr, TLSConfig: conf}, ErrExit: true}
}

type servez struct {
	http.Server
	ErrExit bool
}

func (srv *servez) RunServe(key string) {
	Logn("[_server_]: http server booting... linsten:", key, srv.Server.Addr)
	if srv.Server.TLSConfig != nil {
		if err := srv.Server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			if srv.ErrExit {
				Exit(fmt.Sprintf("[_server_]: server exit error: %s\n", err))
			} else {
				Logn(fmt.Sprintf("[_server_]: server error: %s\n", err))
			}
		}
	} else if err := srv.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		if srv.ErrExit {
			Exit(fmt.Sprintf("[_server_]: server exit error: %s\n", err))
		} else {
			Logn(fmt.Sprintf("[_server_]: server error: %s\n", err))
		}
	}
}

// -----------------------------------------------------------------------------------

// 默认相应函数 http.HandlerFunc(zgg.ServeHTTP)
func (aa *Zgg) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if IsDebug() {
		Logf("[_request]: [%s] %s %s\n", aa.Engine.Name(), rr.Method, rr.URL.String())
	}
	if C.Server.Fxser {
		rw.Header().Set("Xser-Routerz", aa.Engine.Name())
		rw.Header().Set("Xser-Version", AppName+":"+Version)
	}
	aa.Engine.ServeHTTP(rw, rr)
}

// 增加处理函数
// @param key: [method:]action, 如果 method 为空，则默认为 所有请求
func (aa *Zgg) AddRouter(key string, handle HandleFunc) {
	if key == "" {
		if IsDebug() {
			Logf("[_handle_]: %36s    %p\n", "/", handle)
		}
		aa.Engine.Handle("", "", handle)
		return
	}
	// 解析 method 和 action
	method, action, found := key, "", false
	if i := strings.IndexAny(key, " \t"); i >= 0 {
		method, action, found = key[:i], strings.TrimLeft(key[i+1:], " \t"), true
	}
	if !found {
		action = method
		method = ""
	}
	if len(action) > 0 && action[0] == '/' { // 去除 action 前 /
		action = action[1:]
	}
	if C.Server.ApiPath != "" { // 补充 api path
		// action = filepath.Join(C.Server.ApiPath, action)
		action = C.Server.ApiPath + "/" + action
		if action[0] == '/' {
			action = action[1:]
		}
	}
	if method != "" {
		method = strings.ToUpper(method)
	}

	if IsDebug() { // log for debug
		Logf("[_handle_]: %62s  %p  %s\n", method+" /"+action, handle, GetFuncInfo(handle))
	}
	aa.Engine.Handle(method, action, handle)
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// service 管理工具

var _ SvcKit = (*Svc)(nil)

type Svc struct {
	server *Zgg
	svcmap map[string]any
	typmap map[reflect.Type]any
	svclck sync.RWMutex
}

func NewSvcKit(server *Zgg) SvcKit {
	svckit := &Svc{
		server: server,
		svcmap: make(map[string]any),
		typmap: make(map[reflect.Type]any),
	}
	svckit.svcmap["svckit"] = svckit
	svckit.svcmap["server"] = server
	svckit.typmap[reflect.TypeFor[*Svc]()] = svckit
	svckit.typmap[reflect.TypeFor[*Zgg]()] = server
	return svckit
}

func (aa *Svc) Zgg() *Zgg {
	return aa.server
}

func (aa *Svc) Get(key string) any {
	aa.svclck.RLock()
	defer aa.svclck.RUnlock()
	return aa.svcmap[key]
}

func (aa *Svc) Set(key string, val any) SvcKit {
	aa.svclck.Lock()
	defer aa.svclck.Unlock()
	if val != nil {
		// create or update
		aa.svcmap[key] = val
		aa.typmap[reflect.TypeOf(val)] = val
	} else {
		// delete
		val := aa.svcmap[key]
		if val != nil {
			delete(aa.svcmap, key)
			// delete value by type
			for kk, vv := range aa.typmap {
				if vv == val {
					delete(aa.typmap, kk)
					break
				}
			}
		}
	}
	return aa
}

func (aa *Svc) Map() map[string]any {
	aa.svclck.RLock()
	defer aa.svclck.RUnlock()
	ckv := make(map[string]any)
	maps.Copy(ckv, aa.svcmap)
	return ckv
}

func (aa *Svc) toInjName(tType, tField string) string {
	name := fmt.Sprintf("%s.%s", tType, tField)
	if size := len(name); size < 36 {
		name += strings.Repeat(" ", 36-size)
	}
	return name
}

func (aa *Svc) Inj(obj any) SvcKit {
	aa.svclck.RLock()
	defer aa.svclck.RUnlock()
	// 构建注入映射
	tType := reflect.TypeOf(obj).Elem()
	tElem := reflect.ValueOf(obj).Elem()
	for i := 0; i < tType.NumField(); i++ {
		tField := tType.Field(i)
		tagVal := tField.Tag.Get("svckit")
		if tagVal == "" || tagVal == "-" {
			continue // 忽略
		}
		if tagVal == "type" || tagVal == "auto" {
			// 通过 `svckit:'type/auto'` 中的接口匹配注入
			found := false
			for vType, value := range aa.typmap {
				if tField.Type == vType || // 属性是一个接口，判断接口是否可以注入
					tField.Type.Kind() == reflect.Interface && vType.Implements(tField.Type) {
					tElem.Field(i).Set(reflect.ValueOf(value))
					if IsDebug() {
						Logf("[_svckit_]: [inject] %s <- %s\n", aa.toInjName(tType.String(), tField.Name), vType)
					}
					found = true
					break
				}
			}
			if !found {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s <- %s.(type) error, service not found", //
					aa.toInjName(tType.String(), tField.Name), tField.Type)
				if IsDebug() {
					Logn(errstr)
				} else {
					Exit(errstr) // 生产环境，注入失败，则 panic
				}
			}
		} else {
			// 通过 `svckit:'(name)'` 中的 (name) 注入
			val := aa.svcmap[tagVal]
			if val == nil {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s <- %s.(name) error, service not found", //
					aa.toInjName(tType.String(), tField.Name), tagVal)
				if IsDebug() {
					Logn(errstr)
				} else {
					Exit(errstr) // 生产环境，注入失败，则 panic
				}
				continue
			}
			tElem.Field(i).Set(reflect.ValueOf(val))
			if IsDebug() {
				Logf("[_svckit_]: [inject] %s <- %s\n", aa.toInjName(tType.String(), tField.Name), reflect.TypeOf(val))
			}
		}
	}
	return aa
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// template: 模版管理工具

var (
	ErrTplNotFound = errors.New("tpl not found")
)

var _ TplKit = (*Tvc)(nil)

type Tvc struct {
	tpls map[string]*Tpl // 所有模版集合
	lock sync.RWMutex    // 读写锁

	FuncMap template.FuncMap // 支持链式调用
}

func NewTplKit(server *Zgg) TplKit {
	return &Tvc{
		tpls: make(map[string]*Tpl),
	}
}

func (aa *Tvc) Get(key string) *Tpl {
	aa.lock.RLock()
	defer aa.lock.RUnlock()
	return aa.tpls[key]
}

func (tk *Tvc) Render(wr io.Writer, name string, data any) error {
	tpl := tk.Get(name)
	if tpl == nil {
		return ErrTplNotFound
	} else if tpl.Err != nil {
		return tpl.Err
	}
	return tpl.Tpl.Execute(wr, data)
}

func (aa *Tvc) Load(key string, str string) *Tpl {
	aa.lock.Lock()
	defer aa.lock.Unlock()
	if tpl, ok := aa.tpls[key]; ok {
		return tpl
	}
	tpl := &Tpl{}
	tpl.Key = key
	tpl.Txt = str
	tpl.Tpl, tpl.Err = template.New(tpl.Key).Parse(tpl.Txt)
	if tpl.Err == nil {
		tpl.Tpl.Funcs(aa.FuncMap)
	}
	aa.tpls[tpl.Key] = tpl
	return tpl
}

func (aa *Tvc) Preload(dir string) error {
	aa.lock.Lock()
	defer aa.lock.Unlock()
	// 读取 dir 文件夹中 所有的 *.html 文件
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".html") {
			return nil
		}
		key := path
		if idx := strings.IndexRune(path, '/'); idx >= 0 {
			key = path[idx+1:]
		}
		// 读取文件内容
		txt, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tpl := &Tpl{}
		tpl.Key = key
		tpl.Txt = string(txt)
		tpl.Tpl, tpl.Err = template.New(tpl.Key).Parse(tpl.Txt)
		if tpl.Err == nil {
			tpl.Tpl.Funcs(aa.FuncMap)
		}
		aa.tpls[tpl.Key] = tpl
		if IsDebug() {
			Logf("[_preload]: [tplkit] %s", tpl.Key)
		}
		return nil
	})
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// engine: 引擎管理工具

// 基于 map 路由，为更高的性能，单接口而生，是默认的路由
// var _ Engine = (*MapRouter)(nil)

func NewMapRouter(svckit SvcKit) Engine {
	return &MapRouter{
		name:    "zgg-map",
		svckit:  svckit,
		Handles: make(map[string]HandleFunc),
	}
}

type MapRouter struct {
	name    string
	svckit  SvcKit
	Handle_ HandleFunc            // 默认函数，没有找到Action触发
	Handles map[string]HandleFunc // 接口集合

	// https://github.com/puzpuzpuz/xsync
	// 初始化后，map 就不会变更了，可以使用 xsync.Map 获取更高的性能
	// handles *xsync.Map[string, HandleFunc]
}

func (aa *MapRouter) Name() string {
	return aa.name
}

func (aa *MapRouter) Handle(method, action string, handle HandleFunc) {
	if method == "" && action == "" {
		aa.Handle_ = handle // 默认函数
	} else {
		if method == "" {
			method = "GET" // 默认使用 GET
		}
		aa.Handles[method+" /"+action] = handle
	}
}

func (aa *MapRouter) GetHandle(method, action string) (HandleFunc, bool) {
	handle, exist := aa.Handles[method+" /"+action]
	return handle, exist
}

func (aa *MapRouter) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	// 查询并执行业务 Action
	ctx := NewCtx(aa.svckit, rr, rw, aa.name)
	defer ctx.Clear() // 确保取消
	if ctx.Action == "healthz" {
		// 健康健康高优先级， 直接出发检索
		Healthz(ctx)
	} else if handle, exist := aa.GetHandle(rr.Method, ctx.Action); exist {
		// 处理函数
		handle(ctx)
	} else if aa.Handle_ != nil {
		// 默认函数
		aa.Handle_(ctx)
	} else if ctx.Action == "" {
		// 空的操作
		res := &Result{ErrCode: "action-empty", Message: "未指定操作: empty"}
		ctx.JSON(res)
	} else {
		// 无效操作
		res := &Result{ErrCode: "action-unknow", Message: "未指定操作: " + ctx.Action}
		ctx.JSON(res) // 无效操作
	}
}

// -----------------------------------------------------------------------------------

// 基于 http.ServeMux 的路由
// var _ Engine = (*MuxRouter)(nil)

func NewMuxRouter(svckit SvcKit) Engine {
	return &MuxRouter{
		name:   "zgg-mux",
		svckit: svckit,
		Router: http.NewServeMux(),
	}
}

type MuxRouter struct {
	name   string
	svckit SvcKit
	Router *http.ServeMux
}

func (aa *MuxRouter) Name() string {
	return aa.name
}

func (aa *MuxRouter) Handle(method, action string, handle HandleFunc) {
	pattern := "/" + action
	if method != "" {
		pattern = method + " " + pattern
	}
	aa.Router.HandleFunc(pattern, func(rw http.ResponseWriter, rr *http.Request) {
		ctx := NewCtx(aa.svckit, rr, rw, aa.name)
		defer ctx.Clear()
		handle(ctx)
	})
}

func (aa *MuxRouter) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	aa.Router.ServeHTTP(rw, rr)
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------

func RegisterHttpServe(zgg *Zgg) Closed {
	if !HttpServeDef {
		return nil // 不启动默认服务
	}
	if C.Server.Local {
		C.Server.Addr = "127.0.0.1"
	}
	if C.Server.Ptls > 0 && zgg.TLSConf != nil {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Ptls)
		zgg.Servers["(HTTPS)"] = NewServer(zgg, addr, zgg.TLSConf)
	}
	if C.Server.Port > 0 && (zgg.TLSConf == nil || C.Server.Dual) {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Port)
		zgg.Servers["(HTTP1)"] = NewServer(zgg, addr, nil)
	}
	zgg.AddRouter("healthz", Healthz) // 默认注册健康检查
	return nil
}

// -----------------------------------------------------------------------------------

// 健康检查接口
func Healthz(ctx *Ctx) {
	ctx.JSON(&Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
}

// favicon.ico
func Favicon(ctx *Ctx) {
	// 缓存1小时
	ctx.Writer.Header().Set("Cache-Control", "max-age=3600")
	ctx.Writer.Header().Set("Content-Type", "image/x-icon")
	ctx.Writer.Write([]byte{})
}

// -----------------------------------------------------------------------------------

// 请求数据
func ReadBody[T any](rr *http.Request, rb T) (T, error) {
	return rb, json.NewDecoder(rr.Body).Decode(rb)
}

// 请求结构体， 特殊的请求体
type RaData struct {
	Atyp string `json:"type"`
	Data string `json:"data"`
}

// 请求数据
func ReadData(rr *http.Request) (*RaData, error) {
	return ReadBody(rr, &RaData{})
}

// 获取 traceID / 配置 traceID
func GetTraceID(request *http.Request) string {
	traceid := request.Header.Get("X-Request-Id")
	if traceid == "" {
		traceid = GenStr("r", 32) // 创建请求ID, 用于追踪
		request.Header.Set("X-Request-Id", traceid)
	}
	return traceid
}

// 获取 reqType / 配置 reqType
func GetReqType(request *http.Request) string {
	reqtype := request.Header.Get("X-Request-Rt")
	if reqtype == "" {
		reqtype = C.Server.ReqXrtd
		if reqtype != "" {
			request.Header.Set("X-Request-Rt", reqtype)
		}
	}
	return reqtype
}

func GetRemoteIP(req *http.Request) string {
	if ip := req.Header.Get("X-Forwarded-For"); ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
		if ip == "" {
			ip = strings.TrimSpace(req.Header.Get("X-Real-Ip"))
		}
		if ip != "" {
			return ip
		}
	}
	if ip := req.Header.Get("X-Appengine-Remote-Addr"); ip != "" {
		return ip
	}
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}

// request token auth
func TokenAuth(token *string, handle HandleFunc) HandleFunc {
	// 需要验证令牌
	return func(ctx *Ctx) {
		if token == nil || *token == "" {
			handle(ctx) // auth pass
		} else if ktn := ctx.Request.Header.Get("Authorization"); ktn == "Token "+*token {
			handle(ctx) // auth succ
		} else {
			ctx.JSON(&Result{ErrCode: "invalid-token", Message: "无效的令牌"})
		}
	}
}

// merge multi func to one func
func MergeFunc(handles ...HandleFunc) HandleFunc {
	return func(ctx *Ctx) {
		for _, handle := range handles {
			handle(ctx)
			if ctx.IsAbort() {
				return
			}
		}
	}
}

// -----------------------------------------------------------------------------------

// 可获取有权限字段
func FieldValue(target any, field string) any {
	val := reflect.ValueOf(target)
	return val.Elem().FieldByName(field).Interface()
}

// 可设置字段值
func FieldSetVal(target any, field string, value any) {
	val := reflect.ValueOf(target)
	val.Elem().FieldByName(field).Set(reflect.ValueOf(value))
}

// 获取字段, 可夸包获取私有字段
// 闭包原则，原则上不建议使用该方法，因为改方法是在破坏闭包原则
func FieldValue_(target any, field string) any {
	val := reflect.ValueOf(target)
	vap := unsafe.Pointer(val.Elem().FieldByName(field).UnsafeAddr())
	return *(*any)(vap)
}

// -----------------------------------------------------------------------------------

// 创建指针
func Ptr[T any](v T) *T {
	return &v
}

// 键值对
type Ref[K cmp.Ordered, T any] struct {
	Key K
	Val T
}

type BufferPool interface {
	Get() []byte
	Put([]byte)
}

// NewBufferPool 初始化缓冲池
// defCap: 新缓冲区的默认容量（如32KB）
// maxCap: 允许归还的最大容量（如1MB）
func NewBufferPool(defCap, maxCap int) BufferPool {
	if defCap <= 0 {
		defCap = 32 * 1024 // 默认32KB, 现代CPU L1缓存通常为32KB/核
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
		buf = nil
		return
	}
	// 2. 重置缓冲区：保留容量，清空内容（len=0）
	p.pool.Put(buf[:0])
}

// -----------------------------------------------------------------------------------

// 获取 target 中每个字段的属性，注入和 value 属性的字段
// 这只是一个演示的例子，实际开发中，请使用 SvcKit 模块
func FieldInject(target any, value any, tag string, debug bool) bool {
	vType := reflect.TypeOf(value)
	tType := reflect.TypeOf(target).Elem()
	tElem := reflect.ValueOf(target).Elem()
	for i := 0; i < tType.NumField(); i++ {
		tField := tType.Field(i)
		tagVal := tField.Tag.Get(tag)
		if tagVal != "type" && tagVal != "auto" {
			continue // `"tag":"type/auto"` 才可以通过类型注入
		}
		// 判断 vType 是否实现 tField.Type 的接口 // 属性是一个接口，判断接口是否可以注入
		if tField.Type == vType || //
			tField.Type.Kind() == reflect.Interface && vType.Implements(tField.Type) {
			tElem.Field(i).Set(reflect.ValueOf(value))
			if debug {
				Logf("[_inject_]: [succ] %s.%s <- %s", tType, tField.Name, vType)
			}
			return true // 注入成功
		}
	}
	if debug {
		Logf("[_inject_]: [fail] %s not found field.(%s)", tType, vType)
	}
	return false
}

// -----------------------------------------------------------------------------------
