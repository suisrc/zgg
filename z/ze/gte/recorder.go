package gte

import (
	"strings"

	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/gtw"
	"github.com/suisrc/zgg/z/ze/log"
)

func NewRecorder(address string, pty int, tty, body bool, convert gtw.ConvertFunc) gtw.RecordPool {
	if strings.HasPrefix(address, "stdout://") {
		return gtw.NewRecordPool(func(record gtw.IRecord) {
			zc.LogTty(convert(record).ToFmt())
		}, body)
	}
	if strings.HasPrefix(address, "default://") {
		return gtw.NewRecordPool(func(record gtw.IRecord) {
			zc.Logn(convert(record).ToFmt())
		}, body)
	}
	if strings.HasPrefix(address, "file://") {
		writer := log.NewFileWriter(address[7:], 0, tty)
		return gtw.NewRecordPool(func(record gtw.IRecord) {
			writer.Write(append([]byte(convert(record).ToFmt()), '\n'))
		}, body)
	}
	// 其他情况，默认使用 syslog 输出
	network := ""
	if strings.HasPrefix(address, "udp://") {
		network, address = "udp", address[6:]
	} else if strings.HasPrefix(address, "tcp://") {
		network, address = "tcp", address[6:]
	}
	writer := log.NewSyslogWriter(address, network, 0, tty)
	return gtw.NewRecordPool(func(record gtw.IRecord) {
		writer.Write([]byte(convert(record).ToFmt()))
	}, body)

}
