package wsz

// 使用原生 Go 实现 WebSocket 服务器，支持自定义的 Hook 来处理连接和消息
// 在未进行过性能优化的情况下，适合轻量级应用和学习使用

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Handler WebSocket 服务，支持自定义的 NewHook 回调函数来处理连接和消息
type Handler0 struct {
	NewHook NewHookFunc
	Clients sync.Map
}

// ServeHTTP 处理 HTTP 请求，如果是 WebSocket 升级请求，完成握手并处理 WebSocket 连接
func (ss *Handler0) ServeHTTP(wr http.ResponseWriter, rr *http.Request) {
	// z.Logn("[wsserver]: new connection from", rr.RemoteAddr)
	if !IsWebSocket(rr) {
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
	// sender 是一个函数，用于向客户端发送消息，如果发送失败会取消连接
	sender := func(opc byte, msg []byte) error {
		err := WriteServerData(conn, opc, msg) // 发送消息给客户端
		if err != nil {
			LogInfo("[wsserver] send error:", err)
			cancel()
		}
		return err
	}
	// 计算 Sec-WebSocket-Accept 响应头的值
	accept := ComputeAccept(key)
	response := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
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
	// 发送升级协议响应内容给客户端
	if _, err := conn.Write([]byte(response)); err != nil {
		return // 写入错误，关闭连接
	}
	// 启动一个 goroutine 不断读取客户端发送的消息，并将它们发送到 reader 通道
	msgc := make(chan *Message)
	go func() {
		defer close(msgc)
		var msgr *Message = nil // 用于处理分片消息的状态
		for {
			frame, err := ReadServerFrame(buf)
			if err != nil {
				if str := err.Error(); !strings.HasSuffix(str, "EOF") {
					LogInfo("[wsserver] read error: [", accept, "] ", str)
				}
				// 判断 msgc 是否关闭
				msgc <- nil // 发送 nil 消息表示连接关闭
				return      // 读取错误，关闭连接
			}
			// 处理控制帧（可穿插在数据帧分片之间）
			switch frame.OpCode {
			case OpPing:
				// 收到Ping帧必须回复Pong，携带相同载荷
				_ = WriteServerPong(conn, frame.Payload)
			case OpPong:
				// Pong帧可直接忽略，或用于RTT计算
			case OpClose:
				// 收到Close帧, 关闭连接
				// _ = WriteServerClose(writer, 1000, "normal closure")
				msgc <- nil // 发送 nil 消息表示连接关闭
				return      // 主动关闭连接
			default:
				// 非控制帧发送到 msgc 供主循环处理
				if msgr == nil {
					// 首帧：记录opcode
					if frame.OpCode == OpContinuation {
						LogInfo("[wsserver] unexpected continuation frame: [", accept, "]")
						msgc <- nil // 发送 nil 消息表示协议错误，关闭连接
						return      // 协议错误，关闭连接
					}
					msgr = frame // 新的消息帧
				} else {
					// 后续分片：opcode必须是Continuation
					if frame.OpCode != OpContinuation {
						LogInfo("[wsserver] expected continuation frame: [", accept, "]")
						msgc <- nil // 发送 nil 消息表示协议错误，关闭连接
						return      // 协议错误，关闭连接
					}
					// 累积分片数据
					msgr.Payload = append(msgr.Payload, frame.Payload...)
					msgr.IsFinal = frame.IsFinal
				}
				// 如果是最后一帧，发送完整消息到 msgc，并重置 msgr
				if frame.IsFinal {
					msgc <- msgr
					msgr = nil
				}
			}
		}
	}()
	// 处理 WebSocket 连接，监听 reader 通道和 ctx.Done()，如果连接关闭或上下文取消，退出循环
	ServeConn(ctx, accept, msgc, sender, ss.Clients.Load)
}

// ---------------------------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------------------------

// ReadServerFrame 服务器端读取 WebSocket 帧函数
// 参数:
//
//	r: 输入流（TCP连接，建议传入bufio.Reader提升性能）
//
// 返回:
//
//	ReceivedFrame: 解析后的帧数据
//	error: 错误信息
func ReadServerFrame(r io.Reader) (*Message, error) {
	var frame Message
	reader, ok := r.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(r)
	}
	// 1. 读取第一个字节（FIN + RSV + Opcode）
	firstByte, err := reader.ReadByte()
	if err != nil {
		if err == io.EOF {
			return nil, err // 连接关闭，返回 EOF 错误
		}
		return nil, fmt.Errorf("read first byte failed: %w", err)
	}
	frame.IsFinal = (firstByte & 0x80) != 0
	rsv1 := (firstByte & 0x40) != 0
	rsv2 := (firstByte & 0x20) != 0
	rsv3 := (firstByte & 0x10) != 0
	frame.OpCode = firstByte & 0x0F
	// 校验：未协商扩展时RSV位必须为0（支持扩展时可移除）
	if rsv1 || rsv2 || rsv3 {
		return nil, errors.New("non-zero RSV bits received without extension negotiation")
	}
	// 校验 opcode 合法性
	if (frame.OpCode >= 0x3 && frame.OpCode <= 0x7) || (frame.OpCode >= 0xB && frame.OpCode <= 0xF) {
		return nil, errors.New("reserved opcode received")
	}
	// 2. 读取第二个字节（MASK + 长度）
	secondByte, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read second byte failed: %w", err)
	}
	mask := (secondByte & 0x80) != 0
	// 客户端发送的帧必须带掩码，否则是协议错误
	if !mask {
		return nil, errors.New("client sent unmasked frame, protocol violation")
	}
	payloadLen := uint64(secondByte & 0x7F)
	// 3. 读取扩展长度
	switch payloadLen {
	case 126:
		// 2字节大端序长度
		buf := make([]byte, 2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, fmt.Errorf("read 16-bit length failed: %w", err)
		}
		payloadLen = uint64(buf[0])<<8 | uint64(buf[1])
	case 127:
		// 8字节大端序长度
		buf := make([]byte, 8)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, fmt.Errorf("read 64-bit length failed: %w", err)
		}
		payloadLen = uint64(buf[0])<<56 | uint64(buf[1])<<48 |
			uint64(buf[2])<<40 | uint64(buf[3])<<32 |
			uint64(buf[4])<<24 | uint64(buf[5])<<16 |
			uint64(buf[6])<<8 | uint64(buf[7])
		// 最高位必须为0
		if payloadLen > 0x7FFFFFFFFFFFFFFF {
			return nil, errors.New("invalid payload length, MSB set in 64-bit length")
		}
	}
	// 4. 读取4字节掩码
	maskKey := make([]byte, 4)
	if _, err := io.ReadFull(reader, maskKey); err != nil {
		return nil, fmt.Errorf("read mask key failed: %w", err)
	}
	// 5. 读取原始载荷并解掩码
	rawPayload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(reader, rawPayload); err != nil {
			return nil, fmt.Errorf("read payload failed: %w", err)
		}
		// 解掩码：每个字节与掩码循环异或
		frame.Payload = make([]byte, payloadLen)
		for i := uint64(0); i < payloadLen; i++ {
			frame.Payload[i] = rawPayload[i] ^ maskKey[i%4]
		}
	}

	return &frame, nil
}

// WriteServerFrame WebSocket 服务器端帧写入函数
// 参数:
//
//	w: 输出流（TCP 连接）
//	fin: 是否为最后一帧
//	rsv1/rsv2/rsv3: 扩展协议保留位（未启用扩展时必须为 false）
//	opcode: 帧类型
//	payload: 数据载荷
//
// 返回: 错误信息
func WriteServerFrame(w io.Writer, fin bool, rsv1, rsv2, rsv3 bool, opcode byte, payload []byte) error {
	// 1. 基础校验
	if opcode > 0xF || (opcode >= 0x3 && opcode <= 0x7) || (opcode >= 0xB && opcode <= 0xF) {
		return errors.New("invalid opcode")
	}
	// 控制帧不允许分片
	if opcode >= OpClose && !fin {
		return errors.New("control frame cannot be fragmented")
	}
	// 2. 构建帧头部
	header := make([]byte, 0, 10) // 服务器端最大头部长度：1+1+8=10字节（无掩码）
	// 2.1 处理第一个字节：FIN(1) + RSV1(1) + RSV2(1) + RSV3(1) + Opcode(4)
	firstByte := opcode & 0x0F
	if fin {
		firstByte |= 0x80
	}
	if rsv1 {
		firstByte |= 0x40
	}
	if rsv2 {
		firstByte |= 0x20
	}
	if rsv3 {
		firstByte |= 0x10
	}
	header = append(header, firstByte)
	plen := len(payload)
	// 2.2 处理第二个字节：MASK(1, 固定为0) + 长度(7bit)
	secondByte := byte(0) // 服务器端 MASK 位必须为 0
	switch {
	case plen <= 125:
		secondByte |= byte(plen)
		header = append(header, secondByte)
	case plen <= 0xFFFF:
		secondByte |= 126
		header = append(header, secondByte, byte(plen>>8), byte(plen&0xFF))
	default:
		if plen < 0 || uint64(plen) > 0x7FFFFFFFFFFFFFFF {
			return errors.New("payload length exceeds maximum allowed (2^63-1)")
		}
		secondByte |= 127
		header = append(header, secondByte,
			byte(plen>>56), byte(plen>>48), byte(plen>>40), byte(plen>>32),
			byte(plen>>24), byte(plen>>16), byte(plen>>8), byte(plen),
		)
	}
	// 3. 写入头部
	if n, err := w.Write(header); err != nil || n != len(header) {
		return fmt.Errorf("write header failed: %w, wrote %d/%d bytes", err, n, len(header))
	}
	// 4. 直接写入原始 payload（无掩码加密）
	if plen > 0 {
		if n, err := w.Write(payload); err != nil || n != plen {
			return fmt.Errorf("write payload failed: %w, wrote %d/%d bytes", err, n, plen)
		}
	}
	return nil
}

// 发送二进制/文本帧快捷方法
func WriteServerData(w io.Writer, code byte, data []byte) error {
	return WriteServerFrame(w, true, false, false, false, code, data)
}

// 回复 Ping 帧的 Pong 响应
func WriteServerPong(w io.Writer, payload []byte) error {
	return WriteServerFrame(w, true, false, false, false, OpPong, payload)
}

// 发送关闭帧
func WriteServerClose(w io.Writer, statusCode uint16, reason string) error {
	payload := make([]byte, 2+len(reason))
	payload[0] = byte(statusCode >> 8)
	payload[1] = byte(statusCode & 0xFF)
	copy(payload[2:], reason)
	return WriteServerFrame(w, true, false, false, false, OpClose, payload)
}

// ---------------------------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------------------------
// ---------------------------------------------------------------------------------------------------------

// ReadClientFrame 客户端读取WebSocket帧函数
// 参数:
//
//	r: 输入流（通常是TCP连接，建议传入bufio.Reader提升性能）
//
// 返回:
//
//	Message: 解析后的帧数据
//	error: 错误信息
func ReadClientFrame(r io.Reader) (*Message, error) {
	var frame Message
	reader, ok := r.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(r)
	}
	// 1. 读取第一个字节（FIN + RSV + Opcode）
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read first byte failed: %w", err)
	}
	frame.IsFinal = (firstByte & 0x80) != 0
	rsv1 := (firstByte & 0x40) != 0
	rsv2 := (firstByte & 0x20) != 0
	rsv3 := (firstByte & 0x10) != 0
	frame.OpCode = firstByte & 0x0F
	// 校验：未协商扩展时RSV位必须为0（如果需要支持压缩等扩展，可移除该校验）
	if rsv1 || rsv2 || rsv3 {
		return nil, errors.New("non-zero RSV bits received without extension negotiation")
	}
	// 校验opcode合法性
	if (frame.OpCode >= 0x3 && frame.OpCode <= 0x7) || (frame.OpCode >= 0xB && frame.OpCode <= 0xF) {
		return nil, errors.New("reserved opcode received")
	}
	// 2. 读取第二个字节（MASK + 长度）
	secondByte, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read second byte failed: %w", err)
	}
	mask := (secondByte & 0x80) != 0
	// 服务器发送的帧必须不掩码，否则是协议错误
	if mask {
		return nil, errors.New("server sent masked frame, protocol violation")
	}
	payloadLen := uint64(secondByte & 0x7F)
	// 3. 读取扩展长度
	switch payloadLen {
	case 126:
		// 读取2字节大端序长度
		buf := make([]byte, 2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, fmt.Errorf("read 16-bit length failed: %w", err)
		}
		payloadLen = uint64(buf[0])<<8 | uint64(buf[1])
	case 127:
		// 读取8字节大端序长度
		buf := make([]byte, 8)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, fmt.Errorf("read 64-bit length failed: %w", err)
		}
		payloadLen = uint64(buf[0])<<56 | uint64(buf[1])<<48 |
			uint64(buf[2])<<40 | uint64(buf[3])<<32 |
			uint64(buf[4])<<24 | uint64(buf[5])<<16 |
			uint64(buf[6])<<8 | uint64(buf[7])
		// 最高位必须为0
		if payloadLen > 0x7FFFFFFFFFFFFFFF {
			return nil, errors.New("invalid payload length, MSB set in 64-bit length")
		}
	}
	// 4. 读取载荷（服务器帧无掩码，直接读取）
	frame.Payload = make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(reader, frame.Payload); err != nil {
			return nil, fmt.Errorf("read payload failed: %w", err)
		}
	}
	return &frame, nil
}

// ReadClientMessage 读取完整的客户端消息（处理分片和控制帧）
func ReadClientData(reader io.Reader) (byte, []byte, error) {
	var msgr *Message = nil // 用于处理分片消息的状态
	for {
		frame, err := ReadClientFrame(reader)
		if err != nil {
			return 0, nil, err // 读取错误，关闭连接
		}
		// 处理控制帧（可穿插在数据帧分片之间）
		switch frame.OpCode {
		case OpPing:
			// Ping帧可直接忽略
		case OpPong:
			// Pong帧可直接忽略
		case OpClose:
			// 收到Close帧
			return 0, nil, io.EOF // 连接关闭
		default:
			// 非控制帧发送到 msgc 供主循环处理
			if msgr == nil {
				// 首帧：记录opcode
				if frame.OpCode == OpContinuation {
					return 0, nil, errors.New("unexpected continuation frame") // 协议错误，关闭连接
				}
				msgr = frame // 新的消息帧
			} else {
				// 后续分片：opcode必须是Continuation
				if frame.OpCode != OpContinuation {
					return 0, nil, errors.New("expected continuation frame") // 协议错误，关闭连接
				}
				// 累积分片数据
				msgr.Payload = append(msgr.Payload, frame.Payload...)
				msgr.IsFinal = frame.IsFinal
			}
			// 如果是最后一帧，发送完整消息到 msgc，并重置 msgr
			if frame.IsFinal {
				return msgr.OpCode, msgr.Payload, nil
			}
		}
	}
	// return 0, nil, errors.New("unexpected end of message")
}

// WriteClientFrame WebSocket 客户端帧写入函数
// 参数:
//
//	w: 输出流（通常是 TCP 连接）
//	fin: 是否为最后一帧（分片传输时首帧和中间帧设为 false，尾帧设为 true）
//	opcode: 帧类型（使用上述 Op* 常量）
//	payload: 要发送的数据载荷
//
// 返回: 错误信息
func WriteClientFrame(w io.Writer, fin bool, opcode byte, payload []byte) error {
	// 1. 校验 opcode 合法性
	if opcode > 0xF || (opcode >= 0x3 && opcode <= 0x7) || (opcode >= 0xB && opcode <= 0xF) {
		return errors.New("invalid opcode")
	}
	// 2. 构建帧头部
	header := make([]byte, 0, 14) // 最大头部长度: 1字节控制位 + 1字节长度位 + 8字节扩展长度 + 4字节掩码 = 14字节
	// 2.1 处理第一个字节：FIN位(1bit) + RSV1-3(3bit) + Opcode(4bit)
	firstByte := opcode & 0x0F // 取 opcode 低4位
	if fin {
		firstByte |= 0x80 // 设置 FIN 位为1
	}
	// RSV1/RSV2/RSV3 默认为0，如需支持扩展（如压缩）可在此处设置
	header = append(header, firstByte)
	plen := len(payload)
	// 2.2 处理第二个字节：MASK位(1bit) + 载荷长度(7bit)
	secondByte := byte(0x80) // 客户端必须设置 MASK 位为1
	switch {
	case plen <= 125:
		secondByte |= byte(plen)
		header = append(header, secondByte)
	case plen <= 0xFFFF:
		secondByte |= 126
		header = append(header, secondByte, byte(plen>>8), byte(plen&0xFF)) // 2字节大端序长度
	default:
		// 64位长度校验：WebSocket 规范要求最高位为0，因此长度不能超过 2^63-1
		if plen < 0 || uint64(plen) > 0x7FFFFFFFFFFFFFFF {
			return errors.New("payload length exceeds maximum allowed (2^63-1)")
		}
		secondByte |= 127
		header = append(header, secondByte,
			byte(plen>>56), byte(plen>>48), byte(plen>>40), byte(plen>>32),
			byte(plen>>24), byte(plen>>16), byte(plen>>8), byte(plen),
		) // 8字节大端序长度
	}
	// 3. 生成4字节随机掩码（RFC 要求掩码必须是不可预测的随机值）
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return fmt.Errorf("generate mask failed: %w", err)
	}
	header = append(header, mask...)
	// 4. 完整写入头部
	if n, err := w.Write(header); err != nil || n != len(header) {
		return fmt.Errorf("write header failed: %w, wrote %d/%d bytes", err, n, len(header))
	}
	// 5. 掩码加密载荷并写入
	if plen > 0 {
		maskedPayload := make([]byte, plen)
		for i := 0; i < plen; i++ {
			maskedPayload[i] = payload[i] ^ mask[i%4] // 每个字节与掩码循环异或
		}
		if n, err := w.Write(maskedPayload); err != nil || n != plen {
			return fmt.Errorf("write payload failed: %w, wrote %d/%d bytes", err, n, plen)
		}
	}
	return nil
}

// WriteClientData 客户端发送完整消息的简化函数（自动设置 FIN 位）
func WriteClientData(w io.Writer, opcode byte, payload []byte) error {
	return WriteClientFrame(w, true, opcode, payload)
}

// WriteClientClose 客户端发送 Close 帧的简化函数
func WriteClientClose(w io.Writer, code uint16, reason string) error {
	payload := make([]byte, 2+len(reason))
	payload[0] = byte(code >> 8)
	payload[1] = byte(code & 0xFF)
	copy(payload[2:], reason)
	return WriteClientFrame(w, true, OpClose, payload)
}
