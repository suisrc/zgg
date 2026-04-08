package zhe

// 这是一个测试类， 需要屏蔽 init 函数

import (
	"github.com/suisrc/zgg/z"
)

// 初始化方法， 处理 hdl 的而外配置接口
type InitFunc func(hdl *HelloHandler, zgg *z.Zgg)

func init() {
	Init3(nil)
}

func Init3(ifn InitFunc) {
	z.Register("50-hello", func(zgg *z.Zgg) z.Closed {
		hdl := z.Inject(zgg.SvcKit, &HelloHandler{})
		zgg.AddRouter("hello", hdl.hello)

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return func() {
			z.Logn("hdl-hello closed")
		}
	})
	z.Register("zz-world", func(zgg *z.Zgg) z.Closed {
		hdl := zgg.SvcKit.Get("HelloHandler").(*HelloHandler)
		z.GET("world", hdl.world, zgg)
		z.GET("token", z.TokenAuth(z.Ptr("123"), hdl.token), zgg)
		return nil
	})
}

type HelloHandler struct {
	FA any      // 标记不注入，默认
	FB any      `svckit:"-"`         // 标记不注入，默认
	CM *z.Zgg   `svckit:"type"`      // 根据类型自动注入
	SK z.SvcKit `svckit:"type"`      // 根据类型自动注入
	AH any      `svckit:"hdl-hello"` // 根据名称自动注入
	AW any      `svckit:"server"`    // 根据名称自动注入

	TN InitFunc `svckit:"auto"`
	TK z.TplKit `svckit:"auto"` // 根据名称自动注入
}

func (aa *HelloHandler) hello(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "hello!", ErrShow: 1})
}
func (aa *HelloHandler) world(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "world!"})
}
func (aa *HelloHandler) token(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "token!"})
}
