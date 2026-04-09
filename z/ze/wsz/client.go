package wsz

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ReadClientFrame 客户端读取WebSocket帧函数
// 参数:
//
//	r: 输入流（通常是TCP连接，建议传入bufio.Reader提升性能）
//
// 返回:
//
//	ReceivedFrame: 解析后的帧数据
//	error: 错误信息
func ReadClientFrame(r io.Reader) (*ReceivedFrame, error) {
	var frame ReceivedFrame
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
	var msgr *ReceivedFrame = nil // 用于处理分片消息的状态
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
