package wss_test

import (
	"bytes"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/suisrc/zgg/z/ze/wss"
)

type MyHook struct {
}

func (h *MyHook) Receive(code byte, data []byte) (byte, []byte, error) {
	wss.LogInfo("Received:", string(data))
	return wss.OpText, []byte("world"), nil
}

func (h *MyHook) Close() error {
	return nil
}

func NewMyHook(key string, req *http.Request, sender wss.SendFunc, cancel func()) (string, wss.Hook, error) {
	return key, &MyHook{}, nil
}

// go test -v z/ze/wss/ws_test.go -run TestWsHandler1

func TestWsHandler1(t *testing.T) {
	server := wss.NewHandler("", NewMyHook)
	http.HandleFunc("/ws", server.ServeHTTP)
	t.Log("listen on :8888")
	go http.ListenAndServe("127.0.0.1:8888", nil)

	time.Sleep(1 * time.Second)

	// 使用ws 协议链接 ws://127.0.0.1:8888/ws, 并发送 hello 消息，观察服务器日志输出
	// 使用原生golang实现WebSocket客户端
	// 由于标准库没有直接支持WebSocket，需要手动实现握手和数据帧
	// 这里只做简单的握手和发送hello文本帧

	// 1. 建立TCP连接
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	// 2. 发送WebSocket握手请求
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: x3JJHMbDL1EzLkh9GBhXDw==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		t.Fatalf("handshake write error: %v", err)
	}

	// 3. 读取握手响应
	resp := make([]byte, 1024)
	n, err := conn.Read(resp)
	if err != nil {
		t.Fatalf("handshake read error: %v", err)
	}
	if !bytes.Contains(resp[:n], []byte("101 Switching Protocols")) {
		t.Fatalf("handshake failed: %s", resp[:n])
	}

	// 4. 发送WebSocket文本帧 "hello"
	// WebSocket帧格式: FIN=1, opcode=1, mask=1, payload="hello"
	payload := []byte("hello")
	err = wss.WriteClientData(conn, wss.OpText, payload)
	if err != nil {
		t.Fatalf("write ws frame error: %v", err)
	}
	opcode, payload, err := wss.ReadClientData(conn)
	if err != nil {
		t.Fatalf("read ws frame error: %v", err)
	}
	if opcode != wss.OpText || string(payload) != "world" {
		t.Fatalf("unexpected ws response: opcode=%d, payload=%s", opcode, payload)
	}

	t.Log("ws client received:", string(payload))

}
