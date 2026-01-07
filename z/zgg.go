// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// zgg(z? golang google) 核心内容，为简约而生

package z

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
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

	"github.com/suisrc/zgg/z/zc"
)

var (
	AppName = "zgg"
	Version = "v0.0.0"
	AppInfo = "(https://github.com/suisrc/zgg)"

	C = new(struct {
		Server ServerConfig
	})

	// 路由构建器
	Engines = map[string]EngineBuilder{
		"map": NewMapRouter,
		"mux": NewMuxRouter,
	}
)

// 默认配置， Server配置需要内嵌该结构体
type ServerConfig struct {
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

// ----------------------------------------------------------------------------

var _ http.Handler = (*Zgg)(nil)

// 默认服务实体
type Zgg struct {
	Servers map[string]*http.Server
	Closeds []Closed    // 模块关闭函数列表
	TLSConf *tls.Config // Certificates, GetCertificate

	Engine Engine // 路由引擎
	SvcKit SvcKit // 服务工具
	TplKit TplKit // 模版工具
	_abort bool   // 终止标记
}

// ----------------------------------------------------------------------------

// 服务初始化
func (aa *Zgg) ServeInit() bool {
	aa.Servers = map[string]*http.Server{}
	aa.Closeds = []Closed{}
	if aa.SvcKit == nil {
		aa.SvcKit = NewSvcKit(aa, IsDebug())
	}
	if builder, ok := Engines[C.Server.Engine]; !ok {
		zc.Printf("[_router_]: router not found by [-eng %s]\n", C.Server.Engine)
		return false
	} else {
		aa.Engine = builder(aa.SvcKit)
		zc.Printf("[_router_]: build %s.router by [-eng %s]\n", aa.Engine.Name(), C.Server.Engine)
	}
	if aa.TplKit == nil {
		aa.TplKit = NewTplKit(aa, IsDebug())
		if C.Server.TplPath != "" {
			err := aa.TplKit.Preload(C.Server.TplPath)
			if err != nil {
				zc.Printf("[_tplkit_]: Preload error: %v\n", err)
			}
		}
	}
	// -----------------------------------------------
	zc.Println("[register]: register server options...")
	for _, opt := range options {
		if opt.Val == nil {
			continue
		}
		if IsDebug() {
			zc.Println("[register]:", opt.Key)
		}
		cls := opt.Val(aa)
		if cls != nil {
			aa.Closeds = append(aa.Closeds, cls)
		}
		if aa._abort {
			zc.Println("[register]: serve already stop! exit...")
			return false // 退出
		}
	}
	slices.Reverse(aa.Closeds) // 倒序, 后进先出
	return true
}

// 服务终止，注意，这里只会终止模版，不会终止服务， 终止服务，需要调用 hsv.Shutdown
func (aa *Zgg) ServeStop(err ...string) {
	if len(err) > 0 {
		zc.Printl3("[_server_]: serve stop,", strings.Join(err, " "))
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
	zc.Println("[_server_]: http server shutdown")
}

// 启动 HTTP 服务
func (aa *Zgg) RunServe() {
	defer aa.ServeStop() // 停止业务模块， 先停服务，后停模块
	// ------------------------------------------------------------------------
	// 方案1, 不推荐
	// Println("http server Started, Linsten: " + addr)
	// http.ListenAndServe(addr, hdl)
	// ------------------------------------------------------------------------
	// 方案2, 不推荐, 多启动
	// Println("http server Started, Linsten: " + addr)
	// aa.RunningServer(&http.Server{Handler: hdl, Addr: addr})
	// ------------------------------------------------------------------------
	// 方案3， 启动HTTP服务， 并可优雅的终止
	hss := []*http.Server{}
	for key, hsv := range aa.Servers {
		zc.Println("[_server_]: http server started, linsten:", key, hsv.Addr)
		go aa.Execute(hsv)
		hss = append(hss, hsv)
	}
	aa.WaitFor(hss...)
}

// ----------------------------------------------------------------------------

func (aa *Zgg) Execute(hsv *http.Server) {
	if hsv.TLSConfig != nil {
		if err := hsv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			zc.Fatalf("[_server_]: server exit error: %s\n", err)
		}
	} else if err := hsv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zc.Fatalf("[_server_]: server exit error: %s\n", err)
	}
}

func (aa *Zgg) WaitFor(hss ...*http.Server) {
	if len(hss) == 0 {
		zc.Println("[_server_]: no server to wait for")
		return
	}
	ssc := make(chan os.Signal, 1)
	signal.Notify(ssc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ssc
	zc.Println("[_server_]: http server stoping...")
	// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, hsv := range hss {
		if hsv != nil {
			if err := hsv.Shutdown(ctx); err != nil {
				zc.Println("[_server_]: http server shutdown:", err)
			}
		}
	}
}

// ----------------------------------------------------------------------------

// 默认相应函数 http.HandlerFunc(zgg.ServeHTTP)
func (aa *Zgg) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if IsDebug() {
		zc.Printf("[_request]: [%s] %s %s\n", aa.Engine.Name(), rr.Method, rr.URL.String())
		rw.Header().Set("Xser-Routerz", aa.Engine.Name())
	}
	rw.Header().Set("Xser-Version", AppName+":"+Version)
	aa.Engine.ServeHTTP(rw, rr)
}

// 增加处理函数
// @param key: [method:]action, 如果 method 为空，则默认为 所有请求
func (aa *Zgg) AddRouter(key string, handle HandleFunc) {
	if key == "" {
		if IsDebug() {
			zc.Printf("[_handle_]: %36s    %p\n", "/", handle)
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
		action = C.Server.ApiPath + "/" + action
		if action[0] == '/' {
			action = action[1:]
		}
	}
	if method != "" {
		method = strings.ToUpper(method)
	}

	if IsDebug() { // log for debug
		zc.Printf("[_handle_]: %36s    %p\n", method+" /"+action, handle)
	}
	aa.Engine.Handle(method, action, handle)
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------
// service 管理工具

var _ SvcKit = (*SvcKit0)(nil)

type SvcKit0 struct {
	debug  bool
	server *Zgg
	svcmap map[string]any
	typmap map[reflect.Type]any
	svclck sync.RWMutex
}

func NewSvcKit(server *Zgg, debug bool) SvcKit {
	svckit := &SvcKit0{
		debug:  debug,
		server: server,
		svcmap: make(map[string]any),
		typmap: make(map[reflect.Type]any),
	}
	svckit.svcmap["svckit"] = svckit
	svckit.svcmap["server"] = server
	svckit.typmap[reflect.TypeOf(svckit)] = svckit
	svckit.typmap[reflect.TypeOf(server)] = server
	return svckit
}

func (aa *SvcKit0) Zgg() *Zgg {
	return aa.server
}

func (aa *SvcKit0) Get(key string) any {
	aa.svclck.RLock()
	defer aa.svclck.RUnlock()
	return aa.svcmap[key]
}

func (aa *SvcKit0) Set(key string, val any) SvcKit {
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

func (aa *SvcKit0) Map() map[string]any {
	aa.svclck.RLock()
	defer aa.svclck.RUnlock()
	ckv := make(map[string]any)
	maps.Copy(ckv, aa.svcmap)
	return ckv
}

func (aa *SvcKit0) Inj(obj any) SvcKit {
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
					if aa.debug {
						zc.Printf("[_svckit_]: [inject] %s.%s <- %s\n", tType, tField.Name, vType)
					}
					found = true
					break
				}
			}
			if !found {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s.%s <- %s.(type) error, service not found", //
					tType, tField.Name, tField.Type)
				if aa.debug {
					zc.Println(errstr)
				} else {
					zc.Fatalln(errstr) // 生产环境，注入失败，则 panic
				}
			}
		} else {
			// 通过 `svckit:'(name)'` 中的 (name) 注入
			val := aa.svcmap[tagVal]
			if val == nil {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s.%s <- %s.(name) error, service not found", //
					tType, tField.Name, tagVal)
				if aa.debug {
					zc.Println(errstr)
				} else {
					zc.Fatalln(errstr) // 生产环境，注入失败，则 panic
				}
				continue
			}
			tElem.Field(i).Set(reflect.ValueOf(val))
			if aa.debug {
				zc.Printf("[_svckit_]: [inject] %s.%s <- %s\n", tType, tField.Name, reflect.TypeOf(val))
			}
		}
	}
	return aa
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------
// template: 模版管理工具

var (
	ErrTplNotFound = errors.New("tpl not found")
)

var _ TplKit = (*TplKit0)(nil)

type TplKit0 struct {
	debug bool
	tpls  map[string]*Tpl // 所有模版集合
	lock  sync.RWMutex    // 读写锁

	FuncMap template.FuncMap // 支持链式调用
}

func NewTplKit(server *Zgg, debug bool) TplKit {
	return &TplKit0{
		debug: debug,
		tpls:  make(map[string]*Tpl),
	}
}

func (aa *TplKit0) Get(key string) *Tpl {
	aa.lock.RLock()
	defer aa.lock.RUnlock()
	return aa.tpls[key]
}

func (tk *TplKit0) Render(wr io.Writer, name string, data any) error {
	tpl := tk.Get(name)
	if tpl == nil {
		return ErrTplNotFound
	} else if tpl.Err != nil {
		return tpl.Err
	}
	return tpl.Tpl.Execute(wr, data)
}

func (aa *TplKit0) Load(key string, str string) *Tpl {
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

func (aa *TplKit0) Preload(dir string) error {
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
		if aa.debug {
			zc.Printf("[_preload]: [tplkit] %s", tpl.Key)
		}
		return nil
	})
}

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------

// 基于 map 路由，为更高的性能，单接口而生，是默认的路由
var _ Engine = (*MapRouter)(nil)

type MapRouter struct {
	name    string
	svckit  SvcKit
	Handle_ HandleFunc            // 默认函数，没有找到Action触发
	Handles map[string]HandleFunc // 接口集合

	// https://github.com/puzpuzpuz/xsync
	// 初始化后，map 就不会变更了，可以使用 xsync.Map 获取更高的性能
	// handles *xsync.Map[string, HandleFunc]
}

func NewMapRouter(svckit SvcKit) Engine {
	return &MapRouter{
		name:    "zgg-map",
		svckit:  svckit,
		Handles: make(map[string]HandleFunc),
	}
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
	defer ctx.Cancel() // 确保取消
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
var _ Engine = (*MuxRouter)(nil)

type MuxRouter struct {
	name   string
	svckit SvcKit
	Router *http.ServeMux
}

func NewMuxRouter(svckit SvcKit) Engine {
	return &MuxRouter{
		name:   "zgg-mux",
		svckit: svckit,
		Router: http.NewServeMux(),
	}
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
		defer ctx.Cancel()
		handle(ctx)
	})
}

func (aa *MuxRouter) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	aa.Router.ServeHTTP(rw, rr)
}
