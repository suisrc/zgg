package kwdog2

// https://github.com/gojue/ecapture

// 使用 ecapture 进行流量捕获和分析

import (
	"context"
	"io"

	_ "unsafe"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/proc"
)

var _ z.Server = (*KwbeeHandler)(nil)

type KwbeeHandler struct {
	proc proc.Process
	esws io.Writer
}

func (hdl *KwbeeHandler) Name() string {
	return "(KWBEE)"
}

func (hdl *KwbeeHandler) Addr() string {
	return ""
}

func (hdl *KwbeeHandler) RunServe() {
}

func (hdl *KwbeeHandler) Shutdown(ctx context.Context) error {
	return nil
}
