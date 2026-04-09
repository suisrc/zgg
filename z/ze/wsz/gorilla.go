package wsz

// 基于 gorilla/websocket 进行封装, 同步到 wsg 包中， 避免引入第三方依赖
// 建议用于生产环境使用， 以获得更好的性能和稳定性

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z/ze/wsg"
)

// Handler WebSocket 服务，支持自定义的 NewHook 回调函数来处理连接和消息
type Handler1 struct {
	NewHook  NewHookFunc
	Clients  sync.Map
	Upgrader wsg.Upgrader
}

// ServeHTTP 处理 HTTP 请求，如果是 WebSocket 升级请求，完成握手并处理 WebSocket 连接
func (ss *Handler1) ServeHTTP(wr http.ResponseWriter, rr *http.Request) {
	// 升级HTTP连接为WebSocket连接
	conn, err := ss.Upgrader.Upgrade(wr, rr, nil)
	if err != nil {
		http.Error(wr, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(rr.Context())
	defer cancel()
	// sender 是一个函数，用于向客户端发送消息，如果发送失败会取消连接
	sender := func(opc byte, msg []byte) error {
		err := conn.WriteMessage(int(opc), msg)
		if err != nil {
			LogInfo("[wsserver] send error:", err)
			cancel()
		}
		return err
	}
	// 获取一个索引主键， 用于标记链接
	accept, _ := GenUUID()
	// 如果定义了 NewHook 回调函数，调用它创建一个新的 hook
	if ss.NewHook != nil {
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
	// 启动一个 goroutine 不断读取客户端发送的消息，并将它们发送到 reader 通道
	msgc := make(chan *Message)
	go func() {
		defer close(msgc)
		for {
			opc, msg, err := conn.ReadMessage()
			if err != nil {
				if str := err.Error(); !strings.HasSuffix(str, "EOF") {
					LogInfo("[wsserver] read error: [", accept, "] ", str)
				}
				// 判断 msgc 是否关闭
				msgc <- nil // 发送 nil 消息表示连接关闭
				return      // 读取错误，关闭连接
			}
			// 处理控制帧（可穿插在数据帧分片之间）
			switch opc {
			case OpPing:
				// 收到Ping帧必须回复Pong，携带相同载荷
				_ = conn.WriteMessage(OpPong, msg)
			case OpPong:
				// Pong帧可直接忽略，或用于RTT计算
			case OpClose:
				// 收到Close帧, 关闭连接
				msgc <- nil // 发送 nil 消息表示连接关闭
				return      // 主动关闭连接
			case OpText, OpBinary:
				// 非控制帧发送到 msgc 供主循环处理
				msgc <- &Message{OpCode: byte(opc), Payload: msg, IsFinal: true}
			default:
				// 其他未知帧类型忽略
				LogInfo("[wsserver] unknown frame opcode:", opc)
			}
		}
	}()
	// 处理 WebSocket 连接，监听 reader 通道和 ctx.Done()，如果连接关闭或上下文取消，退出循环
	ServeConn(ctx, accept, msgc, sender, ss.Clients.Load)
}
