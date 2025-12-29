// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// zgg(z? golang google) 核心内容，为简约而生

package z

import (
	"bytes"
	"cmp"
	"context"
	crand "crypto/rand"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	mrand "math/rand"
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

	"github.com/suisrc/zgg/z/cfg"
)

var (
	Println = log.Println
	Printf  = log.Printf
	Fatal   = log.Fatal
	Fatalf  = log.Fatalf

	AppName = "zgg"
	Version = "v0.0.0"
	AppInfo = "(https://github.com/suisrc/zgg)"

	C = new(struct {
		Debug  bool
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
	Port    int    `json:"port"  default:"80"`
	CrtFile string `json:"crtfile"`
	KeyFile string `json:"keyfile"`
	ApiPath string `json:"api"`    // root api path
	TplPath string `json:"tpl"`    // templates folder path
	ReqXrtd string `json:"xrt"`    // X-Request-Rt default value
	Engine  string `json:"engine"` // router engine
}

func init() {
	// 注册配置函数
	cfg.Register(C)
}

func LoadConfig() {
	var cfs string
	flag.StringVar(&cfs, "c", "", "config file path")
	flag.BoolVar(&(C.Debug), "debug", false, "debug mode")
	flag.BoolVar(&(C.Server.Local), "local", false, "http server local mode")
	flag.StringVar(&(C.Server.Addr), "addr", "0.0.0.0", "http server addr")
	flag.IntVar(&(C.Server.Port), "port", 80, "http server Port")
	flag.StringVar(&(C.Server.CrtFile), "crt", "", "http server cer file")
	flag.StringVar(&(C.Server.KeyFile), "key", "", "http server key file")
	flag.StringVar(&(C.Server.ApiPath), "api", "", "http server api path")
	flag.StringVar(&(C.Server.ReqXrtd), "xrt", "", "X-Request-Rt default value")
	flag.StringVar(&(C.Server.TplPath), "tpl", "", "templates folder path")
	flag.StringVar(&(C.Server.Engine), "eng", "map", "http server router engine")
	flag.Parse()

	cfg.CFG_ENV = "zgg" // 默认环境变量前缀，ZGG_XXX, 可以取值 cfg.CFG_ENV = Appname
	if cfs != "" {
		Printf("load config files:  %s\n", cfs)
		cfg.MustLoad(strings.Split(cfs, ",")...)
	} else {
		cfg.MustLoad() // 加载默认配置，包括系统环境变量
	}

	cfg.PrintConfig()
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

var _ http.Handler = (*Zgg)(nil)
var _ IServer = (*Zgg)(nil)

// 默认服务实体
type Zgg struct {
	RefSrv IServer // 自身引用
	// HTTP服务实例
	HttpSrv *http.Server
	Closeds []Closed // 模块关闭函数列表
	// 处理函数列表
	Engine Engine // 路由引擎
	SvcKit SvcKit // 服务工具
	TplKit TplKit // 模版工具
	// 标记列表
	FlagStop bool // 终止标记
}

func (aa *Zgg) GetSvcKit() SvcKit {
	return aa.SvcKit
}

func (aa *Zgg) GetTplKit() TplKit {
	return aa.TplKit
}

func (aa *Zgg) GetEngine() Engine {
	return aa.Engine
}

// ----------------------------------------------------------------------------

// 服务初始化
func (aa *Zgg) ServeInit(srv IServer) bool {
	aa.RefSrv = srv
	if aa.RefSrv == nil {
		aa.RefSrv = aa // 默认自身引用
	}
	// -----------------------------------------------
	if aa.SvcKit == nil {
		aa.SvcKit = NewSvcKit(aa.RefSrv, C.Debug)
	}
	if builder, ok := Engines[C.Server.Engine]; ok {
		aa.Engine = builder(aa.SvcKit)
		Printf("[_router_]: build %s.router by [-eng %s]\n", aa.Engine.Name(), C.Server.Engine)
	} else {
		Printf("[_router_]: router not found by [-eng %s]\n", C.Server.Engine)
		return false
	}
	aa.Closeds = make([]Closed, 0)
	// -----------------------------------------------
	if aa.TplKit == nil {
		aa.TplKit = NewTplKit(aa.RefSrv, C.Debug)
	}
	if C.Server.TplPath != "" {
		err := aa.TplKit.Preload(C.Server.TplPath)
		if err != nil {
			Printf("[_tplkit_]: Preload error: %v\n", err)
		}
	}
	// -----------------------------------------------
	Println("[register]: register options...")
	aa.RefSrv.AddRouter("healthz", Healthz) // 默认注册健康检查
	for _, opt := range options {
		if opt.Val == nil {
			continue
		}
		if C.Debug {
			Println("[register]:", opt.Key)
		}
		cls := opt.Val(aa.RefSrv)
		if cls != nil {
			aa.Closeds = append(aa.Closeds, cls)
		}
		if aa.FlagStop {
			Println("[register]: serve already stop! exit...")
			return false // 退出
		}
		slices.Reverse(aa.Closeds) // 倒序, 后进先出
	}
	return true
}

// 服务终止，注意，这里只会终止模版，不会终止服务， 终止服务，需要调用 hsv.Shutdown
func (aa *Zgg) ServeStop() {
	if aa.FlagStop {
		return
	}
	aa.FlagStop = true
	if aa.Closeds != nil {
		for _, cls := range aa.Closeds {
			cls() // 模块关闭
		}
	}
}

// 启动 HTTP 服务
func (aa *Zgg) RunAndWait(hdl http.HandlerFunc) {
	// ------------------------------------------------------------------------
	// Printf("http server Started, Linsten: %s:%d\n", srv.Addr, srv.Port)
	// http.ListenAndServe(fmt.Sprintf("%s:%d", addr, port), handler) // 启动HTTP服务
	// ------------------------------------------------------------------------
	// 启动HTTP服务， 并可优雅的终止
	hsv := &http.Server{Addr: fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Port), Handler: hdl}
	go func() {
		if C.Server.Local {
			Printf("http server started, linsten: %s:%d (LOCAL)\n", "127.0.0.1", C.Server.Port)
			if err := hsv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				Fatalf("Linsten: %s\n", err)
			}
		} else if C.Server.CrtFile == "" || C.Server.KeyFile == "" {
			Printf("http server started, linsten: %s:%d (HTTP)\n", C.Server.Addr, C.Server.Port)
			if err := hsv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				Fatalf("Linsten: %s\n", err)
			}
		} else {
			if C.Server.Port == 80 {
				C.Server.Port = 443 // 默认使用443端口
			}
			Printf("http server started, linsten: %s:%d (HTTPS)\n", C.Server.Addr, C.Server.Port)
			if err := hsv.ListenAndServeTLS(C.Server.CrtFile, C.Server.KeyFile); err != nil && err != http.ErrServerClosed {
				Fatalf("Linsten: %s\n", err)
			}
		}
	}()
	ssc := make(chan os.Signal, 1)
	signal.Notify(ssc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ssc
	Println("http server stoping...")
	// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := hsv.Shutdown(ctx); err != nil {
		Fatal("http server shutdown:", err)
	}
	aa.RefSrv.ServeStop() // 停止业务模块， 先停服务，后停模块
	Println("http server shutdown")
}

// ----------------------------------------------------------------------------

// 默认相应函数 http.HandlerFunc
func (aa *Zgg) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if C.Debug {
		Printf("[_request]: [%s] %s %s\n", aa.Engine.Name(), rr.Method, rr.URL.String())
		rw.Header().Set("Serv-Handler", aa.Engine.Name())
	}
	rw.Header().Set("Serv-Version", AppName+":"+Version)
	aa.Engine.ServeHTTP(rw, rr)
}

// ----------------------------------------------------------------------------

/**
 * 增加处理函数
 * @param key: [method:]action, 如果 method 为空，则默认为 所有请求
 */
func (aa *Zgg) AddRouter(key string, handle HandleFunc) {
	if key == "" {
		if C.Debug {
			Printf("[_handle_]: %36s    %v\n", "/", handle)
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

	if C.Debug { // log for debug
		Printf("[_handle_]: %36s    %v\n", method+" /"+action, handle)
	}
	aa.Engine.Handle(method, action, handle)
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------
// service 管理工具

var _ SvcKit = (*SvcKit0)(nil)

type SvcKit0 struct {
	debug  bool
	server IServer
	svcmap map[string]any
	typmap map[reflect.Type]any
	svclck sync.RWMutex
}

func NewSvcKit(server IServer, debug bool) SvcKit {
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

func (aa *SvcKit0) Srv() IServer {
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
						Printf("[_svckit_]: [inject] %s.%s <- %s\n", tType, tField.Name, vType)
					}
					found = true
					break
				}
			}
			if !found {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s.%s <- %s.(type) error not found", //
					tType, tField.Name, tField.Type)
				if aa.debug {
					Println(errstr)
				} else {
					Fatal(errstr) // 生产环境，注入失败，则 panic
				}
			}
		} else {
			// 通过 `svckit:'[name]'` 中的 [name] 注入
			val := aa.svcmap[tagVal]
			if val == nil {
				errstr := fmt.Sprintf("[_svckit_]: [inject] %s.%s <- %s.[name] error not found", //
					tType, tField.Name, tagVal)
				if aa.debug {
					Println(errstr)
				} else {
					Fatal(errstr) // 生产环境，注入失败，则 panic
				}
				continue
			}
			tElem.Field(i).Set(reflect.ValueOf(val))
			if aa.debug {
				Printf("[_svckit_]: [inject] %s.%s <- %s\n", tType, tField.Name, reflect.TypeOf(val))
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

func NewTplKit(server IServer, debug bool) TplKit {
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
			Printf("[_preload]: [tplkit] %s", tpl.Key)
		}
		return nil
	})
}

// ----------------------------------------------------------------------------
// ----------------------------------------------------------------------------

// 创建指针
func Ptr[T any](v T) *T {
	return &v
}

// 键值对
type Ref[K cmp.Ordered, T any] struct {
	Key K
	Val T
}

// map to struct
func Mts[T any](target T, source map[string][]string, tagkey string) (T, error) {
	return cfg.Map2ToStruct(target, source, tagkey)
}

// 随机生成字符串， 0~f, 首字母不是 bb
// @param bb 首字母
func GenStr(bb string, ll int) string {
	str := []byte("0123456789abcdef")
	buf := make([]byte, ll-len(bb))
	for i := range buf {
		buf[i] = str[mrand.Intn(len(str))]
	}
	return bb + string(buf)
}

// 生成UUIDv4
func GenUUIDv4() (string, error) {
	// 1. 生成16个随机字节
	uuid := make([]byte, 16)
	if _, err := crand.Read(uuid); err != nil {
		return "", err // 随机数生成失败
	}

	// 2. 设置UUID版本和变体
	uuid[6] = (uuid[6] & 0x0F) | 0x40 // 第13位：0100（V4）
	uuid[8] = (uuid[8] & 0x3F) | 0x80 // 第17位：10xx（变体规范）

	// 3. 格式化为UUID字符串（8-4-4-4-12）
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

func UnicodeToRunes(srs ...[]byte) ([]byte, error) {
	var rst bytes.Buffer
	for _, src := range srs {
		n := len(src)
		for i := 0; i < n; {
			// 匹配\u转义序列：检查是否有足够字节，且当前为'\\'、下一个为'u'
			if i+1 < n && src[i] == '\\' && src[i+1] == '\\' {
				// 转义序列
				rst.WriteByte(src[i])
				rst.WriteByte(src[i+1])
				i += 2
			} else if i+5 <= n && src[i] == '\\' && src[i+1] == 'u' {
				// 提取4位十六进制字节（i+2到i+5）
				bts := src[i+2 : i+6]
				// 手动解析十六进制为Unicode码点（uint16范围：0~65535）
				// code, size := utf8.DecodeRune(bts)
				// if size == 0 {
				// 	return nil, fmt.Errorf("invalid unicode, \\u%s", string(bts))
				// }
				var code rune
				for _, b := range bts {
					code = code << 4
					if b >= '0' && b <= '9' {
						code += rune(b - '0')
					} else if b >= 'a' && b <= 'f' {
						code += rune(b - 'a' + 10)
					} else if b >= 'A' && b <= 'F' {
						code += rune(b - 'A' + 10)
					} else {
						return nil, fmt.Errorf("invalid unicode, \\u%s", string(bts))
					}
				}
				// 将码点转为UTF-8字节并写入结果（rune自动转UTF-8）
				rst.WriteRune(code)
				// 跳过已处理的6个字节（\uXXXX）
				i += 6
			} else {
				// 普通字节直接写入结果
				rst.WriteByte(src[i])
				i++
			}
		}
	}
	return rst.Bytes(), nil
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

// 健康检查接口
func Healthz(ctx *Ctx) bool {
	return ctx.JSON(&Result{Success: true, Data: time.Now().Format("2006-01-02 15:04:05")})
}

// ----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------------
// -----------------------------------------------------------------------------------

type BufferPool interface {
	Get() []byte
	Put([]byte)
}

// NewBufferPool 初始化缓冲池
// defCap: 新缓冲区的默认容量（如4KB）
// maxCap: 允许归还的最大容量（如1MB）
func NewBufferPool(defCap, maxCap int) BufferPool {
	if defCap <= 0 {
		defCap = 32 * 1024 // 默认64KB
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

// -----------------------------------------------------------------------------------
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
				Printf("[_inject_]: [succ] %s.%s <- %s", tType, tField.Name, vType)
			}
			return true // 注入成功
		}
	}
	if debug {
		Printf("[_inject_]: [fail] %s not found field.(%s)", tType, vType)
	}
	return false
}
