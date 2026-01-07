// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package z

import (
	"bytes"
	"cmp"
	crand "crypto/rand"
	"flag"
	"fmt"
	mrand "math/rand"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/suisrc/zgg/z/zc"
)

func IsDebug() bool {
	return zc.C.Debug
}

var HttpServeDef = true // 启动默认 http 服务

// 注册默认方法
func Initializ() {
	// 注册配置函数
	zc.Register(C)

	flag.BoolVar(&(zc.C.Debug), "debug", false, "debug mode")
	flag.BoolVar(&(zc.C.Print), "print", false, "print mode")
	flag.BoolVar(&(C.Server.Local), "local", false, "http server local mode")
	flag.StringVar(&(C.Server.Addr), "addr", "0.0.0.0", "http server addr")
	flag.IntVar(&(C.Server.Port), "port", 80, "http server Port")
	flag.IntVar(&(C.Server.Ptls), "ptls", 443, "https server Port")
	flag.BoolVar(&(C.Server.Dual), "dual", false, "running http and https server")
	flag.StringVar(&(C.Server.Engine), "eng", "map", "http server router engine")
	flag.StringVar(&(C.Server.ApiPath), "api", "", "http server api path")
	flag.StringVar(&(C.Server.TplPath), "tpl", "", "templates folder path")
	flag.StringVar(&(C.Server.ReqXrtd), "xrt", "", "X-Request-Rt default value")

	//  register default serve
	if HttpServeDef {
		Register("90-server", RegisterDefaultHttpServe)
	}
}

func RegisterDefaultHttpServe(zgg *Zgg) Closed {
	if C.Server.Local {
		C.Server.Addr = "127.0.0.1"
	}
	if C.Server.Ptls > 0 && zgg.TLSConf != nil {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Ptls)
		zgg.Servers["(HTTPS)"] = &http.Server{Handler: zgg, Addr: addr, TLSConfig: zgg.TLSConf}
		// zc.Println("[register]: register http server, (HTTPS)", addr)
	}
	if C.Server.Port > 0 && (zgg.TLSConf == nil || C.Server.Dual) {
		addr := fmt.Sprintf("%s:%d", C.Server.Addr, C.Server.Port)
		zgg.Servers["(HTTP1)"] = &http.Server{Handler: zgg, Addr: addr}
		// zc.Println("[register]: register http server, (HTTP1)", addr)
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
	return zc.Map2ToStruct(target, source, tagkey)
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

// -----------------------------------------------------------------------------------

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
				zc.Printf("[_inject_]: [succ] %s.%s <- %s", tType, tField.Name, vType)
			}
			return true // 注入成功
		}
	}
	if debug {
		zc.Printf("[_inject_]: [fail] %s not found field.(%s)", tType, vType)
	}
	return false
}
