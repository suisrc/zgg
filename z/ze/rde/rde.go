package rde

import (
	"github.com/suisrc/zgg/z"
)

// 全局 key 转换函数, 可以通过启动的时候，全局替换掉
// 需要注意， 必须在 svckit 中注册一个 "rde-helper" 的 Helper 实例， 以供 DirRouter 和 HstRouter 使用

type Helper interface {
	KeyGetter(key string) (string, error)
	NewRouter(svckit z.SvcKit) z.Engine
}

func init() {
	z.Engines["dir"] = NewDirRouter
	z.Engines["hst"] = NewHstRouter
}
