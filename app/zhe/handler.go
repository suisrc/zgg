package zhe

// 这是一个测试类， 需要屏蔽 init 函数

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/z/ze/wsz"
)

// 初始化方法， 处理 hdl 的而外配置接口
type InitFunc func(hdl *HelloHandler, zgg *z.Zgg)

func init() {
	Init3(nil)
}

func Init3(ifn InitFunc) {
	z.Register("50-hello", func(zgg *z.Zgg) z.Closed {
		hdl := z.Inject(zgg.SvcKit, &HelloHandler{})
		hdl.WS = wsz.NewHandler(hdl.NewHook, 1)
		zgg.AddRouter("hello", hdl.hello)
		zgg.AddRouter("ws", hdl.wsworker)

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
	WS http.Handler
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

func (aa *HelloHandler) wsworker(zrc *z.Ctx) {
	aa.WS.ServeHTTP(zrc.Writer, zrc.Request)
}

func (hdl *HelloHandler) NewHook(key string, req *http.Request, sender wsz.SendFunc, cancel func()) (string, wsz.Hook, error) {
	return key, hdl, nil
}

// wsz.Hook 接口实现
func (hdl *HelloHandler) Close() error {
	return nil
}

func (hdl *HelloHandler) Receive(code byte, data []byte) (byte, []byte, error) {
	if bts, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		data = bts // 解码成功，使用解码后的数据
	}
	rmap := map[string]any{}
	if err := json.Unmarshal(data, &rmap); err != nil {
		z.Logn("[_hello__]: [not json]", string(data))
		return 0, nil, nil
	}
	delete(rmap, "level")
	delete(rmap, "time")
	z.Logn("[_hello__]:", zc.ToStrText(rmap, "Description", "message"))
	return 0, nil, nil
}
