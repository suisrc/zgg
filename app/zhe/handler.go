package zhe

// 这是一个测试类， 需要屏蔽 init 函数

import (
	"github.com/suisrc/zgg/z"
)

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *HelloApi, zgg *z.Zgg)

func init() {
	Init3(nil)
}

func Init3(ifn InitializFunc) {
	z.Register("50-hello", func(zgg *z.Zgg) z.Closed {
		api := z.Inject(zgg.SvcKit, &HelloApi{})
		zgg.AddRouter("hello", api.hello)

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return func() {
			z.Println("api-hello closed")
		}
	})
	z.Register("zz-world", func(zgg *z.Zgg) z.Closed {
		api := zgg.SvcKit.Get("HelloApi").(*HelloApi)
		z.GET("world", api.world, zgg)
		z.GET("token", z.TokenAuth(z.Ptr("123"), api.token), zgg)
		return nil
	})
}

type HelloApi struct {
	FA any      // 标记不注入，默认
	FB any      `svckit:"-"`         // 标记不注入，默认
	CM *z.Zgg   `svckit:"type"`      // 根据类型自动注入
	SK z.SvcKit `svckit:"type"`      // 根据类型自动注入
	AH any      `svckit:"api-hello"` // 根据名称自动注入
	AW any      `svckit:"server"`    // 根据名称自动注入

	TN                                          InitializFunc `svckit:"auto"`
	TK11111111111111111111111111111111111111111 z.TplKit      `svckit:"auto"` // 根据名称自动注入
}

func (aa *HelloApi) hello(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "hello!", ErrShow: 1})
}
func (aa *HelloApi) world(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "world!"})
}
func (aa *HelloApi) token(zrc *z.Ctx) {
	zrc.JSON(&z.Result{Success: true, Data: "token!"})
}
