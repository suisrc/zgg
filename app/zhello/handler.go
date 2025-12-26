package zhello

// 这是一个测试类， 需要屏蔽 init 函数

import (
	"github.com/suisrc/zgg/z"
)

func init() {
	Init3()
}

func Init3() {
	z.Register("01-hello", func(srv z.IServer) z.Closed {
		api := z.Inject(srv.GetSvcKit(), &HelloApi{})
		srv.AddRouter("hello", api.hello)
		return func() {
			z.Println("api-hello closed")
		}
	})
	z.Register("zz-world", func(srv z.IServer) z.Closed {
		api := srv.GetSvcKit().Get("HelloApi").(*HelloApi)
		z.GET("world", api.world, srv)
		z.GET("token", z.TokenAuth(z.Ptr("123"), api.token), srv)
		return nil
	})
}

type HelloApi struct {
	FA any       // 标记不注入，默认
	FB any       `svckit:"-"`         // 标记不注入，默认
	CM z.IServer `svckit:"type"`      // 根据类型自动注入
	SK z.SvcKit  `svckit:"type"`      // 根据类型自动注入
	AH any       `svckit:"api-hello"` // 根据名称自动注入
	AW any       `svckit:"api-world"` // 根据名称自动注入
	TK z.TplKit  `svckit:"auto"`      // 根据名称自动注入
}

func (aa *HelloApi) hello(zrc *z.Ctx) bool {
	return zrc.JSON(&z.Result{Success: true, Data: "hello!", ErrShow: 1})
}
func (aa *HelloApi) world(zrc *z.Ctx) bool {
	return zrc.JSON(&z.Result{Success: true, Data: "world!"})
}
func (aa *HelloApi) token(zrc *z.Ctx) bool {
	return zrc.JSON(&z.Result{Success: true, Data: "token!"})
}
