package wsz

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"net/http"

	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/wsg"
)

// 定义 WebSocket 操作码常量（RFC 6455）
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA

	KeyGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

var (
	LogInfo = zc.Logn
	GenUUID = zc.GenUUIDv4
)

// Message 接收到的帧结构
type Message struct {
	OpCode  byte   // 帧类型
	Payload []byte // 解掩码后的载荷数据
	IsFinal bool   // 是否为最后一帧
}

// Hook 是一个接口，定义了处理 websocket 消息的回调函数
type Hook interface {
	Receive(byte, []byte) (byte, []byte, error)
	Close() error
}

// SendFunc 是一个函数类型，定义了向客户端发送消息的回调函数
type SendFunc func(byte, []byte) error

// NewHookFunc 是一个函数类型，定义了创建新的 Hook 的回调函数
type NewHookFunc func(key string, req *http.Request, sender SendFunc, cancel func()) (string, Hook, error)

// NewHandler 创建一个新的 Handler 实例
func NewHandler(newHook NewHookFunc, kind int) http.Handler {
	switch kind {
	case 0:
		return &Handler0{
			NewHook: newHook,
		}
	case 1:
		return &Handler1{
			NewHook: newHook,
			Upgrader: wsg.Upgrader{
				ReadBufferSize:  4096,
				WriteBufferSize: 4096,
				CheckOrigin: func(r *http.Request) bool {
					return true // 开发环境允许所有跨源，生产环境需替换为严格校验
				},
			},
		}
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------------------------------------

// IsWebSocket 判断请求是否为 WebSocket 升级请求
func IsWebSocket(r *http.Request) bool {
	return zc.EqualFold(r.Header.Get("Connection"), "upgrade") && zc.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// ComputeAccept 计算 Sec-WebSocket-Accept 响应头的值
func ComputeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + KeyGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ServeConn 处理 WebSocket 连接，监听 reader 通道和 ctx.Done()，如果连接关闭或上下文取消，退出循环
func ServeConn(ctx context.Context, ckey string, reader <-chan *Message, sender SendFunc, getter func(key any) (any, bool)) {
	for {
		// 监听 reader 和 ctx.Done()，如果连接关闭或上下文取消，退出循环
		select {
		case <-ctx.Done():
			return // 上游上下文终止，退出循环
		case msg := <-reader:
			if msg == nil {
				return // 读取通道关闭，退出循环
			}
			if getter == nil {
				LogInfo("[wsserver] no cache function for accept: [", ckey, "] <- ", string(msg.Payload))
				continue // 没有提供 cache 函数，无法处理消息
			}
			hh, ok := getter(ckey)
			if !ok {
				LogInfo("[wsserver] no hook for accept: [", ckey, "] <- ", string(msg.Payload))
				continue // 没有找到对应的 hook，忽略消息
			}
			hook, ok := hh.(Hook)
			if !ok {
				LogInfo("[wsserver] invalid hook type for accept: [", ckey, "] <- ", string(msg.Payload))
				continue // hook 类型不正确，忽略消息
			}
			opcode, payload, err := hook.Receive(msg.OpCode, msg.Payload)
			if err != nil {
				LogInfo("[wsserver] hook receive error: [", ckey, "] ", err)
				return // 处理业务错误，关闭连接
			}
			if len(payload) == 0 {
				continue // 如果 hook 没有返回消息，继续等待下一条消息
			}
			if err := sender(opcode, payload); err != nil {
				LogInfo("[wsserver] send error: [", ckey, "] ", err)
				return // 写入错误，关闭连接
			}
		}
	}
}
