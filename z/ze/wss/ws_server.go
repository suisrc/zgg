package wss

// 一个简单的 WebSocket 服务器示例，接受 WebSocket 连接，并回显客户端发送的文本消息

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z/zc"
)

var (
	LogInfo = zc.Logn
	GenUUID = zc.GenUUIDv4
)

// Hook 是一个接口，定义了处理 websocket 消息的回调函数
type Hook interface {
	Receive([]byte) ([]byte, error)
	Close() error
}

// NewHookFunc 是一个函数类型，定义了创建新的 Hook 的回调函数
type NewHookFunc func(key string, req *http.Request, sender func([]byte) error, cancel func()) (string, Hook, error)

// NewHandler 创建一个新的 Handler 实例
func NewHandler(wsToken string, newHook NewHookFunc) *Handler {
	return &Handler{
		WsToken: wsToken,
		NewHook: newHook,
	}
}

// WsServer websocket 服务，支持自定义的 NewHook 回调函数来处理连接和消息
type Handler struct {
	WsToken string
	NewHook NewHookFunc
	Clients sync.Map
}

// IsWebsocket 判断请求是否为 websocket 升级请求
func (ss *Handler) IsWebsocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// ComputeAccept 计算 Sec-WebSocket-Accept 响应头的值，使用客户端发送的 Sec-WebSocket-Key 和服务器的 WsToken 进行哈希计算
func (ss *Handler) ComputeAccept(key string) string {
	if ss.WsToken == "" {
		ss.WsToken, _ = GenUUID() // 随机生成一个 token
	}
	h := sha1.New()
	h.Write([]byte(key + ss.WsToken))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ServeHTTP 处理 HTTP 请求，如果是 websocket 升级请求，完成握手并处理 websocket 连接
func (ss *Handler) ServeHTTP(wr http.ResponseWriter, rr *http.Request) {
	if !ss.IsWebsocket(rr) {
		http.Error(wr, "upgrade required", http.StatusBadRequest)
		return
	}
	// 客户端随机生成的验证字符串
	key := rr.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(wr, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}
	// 升级协议，获取底层连接
	hj, ok := wr.(http.Hijacker)
	if !ok {
		http.Error(wr, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(wr, "hijack failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(rr.Context())
	defer cancel()
	// 计算 Sec-WebSocket-Accept 响应头的值
	accept := ss.ComputeAccept(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	// 如果定义了 NewHook 回调函数，调用它创建一个新的 hook
	if ss.NewHook != nil {
		// sender 是一个函数，用于向客户端发送消息，如果发送失败会取消连接
		sender := func(msg []byte) error {
			err := ss.write(conn, 0x1, msg) // 发送文本消息给客户端
			if err != nil {
				LogInfo("[wsserver] send error:", err)
				cancel()
			}
			return err
		}
		// 通过 NewHook 创建一个新的 hook，并将它存储在 clients 中，连接关闭时删除它
		ckey, hook, err := ss.NewHook(accept, rr, sender, cancel)
		if err != nil {
			http.Error(wr, "failed to create hook", http.StatusInternalServerError)
			return
		}
		if ckey != "" && hook != nil {
			ss.Clients.Store(ckey, hook)
			defer ss.Clients.Delete(ckey)
			defer hook.Close()
			accept = ckey // 使用自定义的 ckey 作为 accept 键
		}
	}
	// 发送升级协议响应内容给客户端
	if _, err := conn.Write([]byte(response)); err != nil {
		return // 写入错误，关闭连接
	}
	// 处理 WebSocket 连接，监听 reader 通道和 ctx.Done()，如果连接关闭或上下文取消，退出循环
	ss.serve(ctx, accept, conn, buf)
}

// serve 处理 WebSocket 连接
func (ss *Handler) serve(ctx context.Context, accept string, writer io.Writer, reader io.Reader) {
	type Msg struct {
		opcode  byte
		payload []byte
	}
	// 启动一个 goroutine 不断读取客户端发送的消息，并将它们发送到 reader 通道
	readerCh := make(chan *Msg)
	defer close(readerCh)
	go func() {
		for {
			if opcode, payload, err := ss.read(reader); err != nil {
				readerCh <- nil // 发送 nil 消息表示读取错误，关闭连接
				LogInfo("[wsserver] read error: [", accept, "] ", err)
				return // 读取错误，关闭连接
			} else {
				readerCh <- &Msg{opcode, payload}
			}
		}
	}()
	for {
		// 监听 ss.read 和 ctx.Done()，如果连接关闭或上下文取消，退出循环
		select {
		case <-ctx.Done():
			return // 上游上下文终止，退出循环
		case msg := <-readerCh:
			if msg == nil {
				return // 读取通道关闭，退出循环
			}
			switch msg.opcode {
			case 0x1: // text
				hh, ok := ss.Clients.Load(accept)
				if !ok {
					LogInfo("[wsserver] no hook for accept: [", accept, "] <- ", string(msg.payload))
					continue // 没有找到对应的 hook，忽略消息
				}
				hook, ok := hh.(Hook)
				if !ok {
					LogInfo("[wsserver] invalid hook type for accept: [", accept, "] <- ", string(msg.payload))
					continue // hook 类型不正确，忽略消息
				}
				bts, err := hook.Receive(msg.payload)
				if err != nil {
					LogInfo("[wsserver] hook receive error: [", accept, "] ", err)
					return // 处理业务错误，关闭连接
				}
				if len(bts) == 0 {
					// 如果 hook 没有返回消息，继续等待下一条消息
					continue
				}
				if err := ss.write(writer, 0x1, bts); err != nil {
					LogInfo("[wsserver] send error: [", accept, "] ", err)
					return // 写入错误，关闭连接
				}
			case 0x8: // close
				_ = ss.write(writer, 0x8, nil)
				return // 连接关闭
			case 0x9: // ping
				_ = ss.write(writer, 0xA, msg.payload) // 回复 pong 消息
			case 0xA: // pong
				continue
			default:
				continue // 不处理其他类型的消息
			}
		}
	}
}

func (ss *Handler) read(r io.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return
	}
	fin := header[0]&0x80 != 0
	opcode = header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7F)
	if !fin {
		err = fmt.Errorf("fragmented frames not supported")
		return
	}
	if !masked {
		err = fmt.Errorf("client frames must be masked")
		return
	}
	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		length = int(ext[0])<<8 | int(ext[1])
	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		length = int(ext[4]) | int(ext[5])<<8 | int(ext[6])<<16 | int(ext[7])<<24
	}
	maskKey := make([]byte, 4)
	if _, err = io.ReadFull(r, maskKey); err != nil {
		return
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	for i := 0; i < length; i++ {
		payload[i] ^= maskKey[i%4]
	}
	return
}

func (ss *Handler) write(w io.Writer, opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	plen := len(payload)
	switch {
	case plen <= 125:
		header = append(header, byte(plen))
	case plen <= 0xFFFF:
		header = append(header, 126, byte(plen>>8), byte(plen))
	default:
		header = append(header, 127,
			byte(plen>>56), byte(plen>>48), byte(plen>>40), byte(plen>>32),
			byte(plen>>24), byte(plen>>16), byte(plen>>8), byte(plen))
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if plen > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}
