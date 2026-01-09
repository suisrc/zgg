// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package z

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	AppName = "zgg"
	Version = "v0.0.0"
	AppInfo = "(https://github.com/suisrc/zgg)"

	HttpServeDef = true // 启动默认 http 服务?
)

func PrintVersion() {
	println(AppName, Version, AppInfo)
}

func RegisterDefaultHttpServe(zgg *Zgg) Closed {
	if !HttpServeDef {
		return nil // 不启动默认服务
	}
	if C.Server.Local {
		C.Server.Addr = "127.0.0.1"
	}
	if C.Server.Ptls > 0 && zgg.TLSConf != nil {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Ptls)
		zgg.Servers["(HTTPS)"] = &http.Server{Handler: zgg, Addr: addr, TLSConfig: zgg.TLSConf}
	}
	if C.Server.Port > 0 && (zgg.TLSConf == nil || C.Server.Dual) {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Port)
		zgg.Servers["(HTTP1)"] = &http.Server{Handler: zgg, Addr: addr}
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
		buf = nil
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
