package kwdog2

// https 透传

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"strings"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/tlsx"
)

type KwsslConfig struct {
	Disabled bool              `json:"disabled"`
	AddrPort string            `json:"port" default:"0.0.0.0:443"`
	Routers  map[string]string `json:"routers"`               // 其他路由
	MaxConn  int               `json:"maxconn" default:"100"` // 最大并发数
}

// 初始化方法， 处理 api 的而外配置接口 443
type InitKwsslFunc func(api *Kwssl2Api, zgg *z.Zgg)

func InitKwssl(ifn InitKwsslFunc) {

	flag.BoolVar(&C.Kwssl2.Disabled, "s2disabled", true, "是否禁用kwssl2")
	flag.StringVar(&C.Kwssl2.AddrPort, "s2port", "0.0.0.0:443", "代理服务器地址和端口")
	flag.Var(z.NewStrMap(&C.Kwssl2.Routers, z.HM{}), "s2rmap", "其他服务转发")

	z.Register("13-kwssl2", func(zgg *z.Zgg) z.Closed {
		if C.Kwssl2.Disabled || len(C.Kwssl2.Routers) == 0 {
			z.Logn("[_kwssl2_]: disabled")
			return nil
		}
		api := &Kwssl2Api{
			Address: C.Kwssl2.AddrPort,
			Routers: C.Kwssl2.Routers,
			MaxConn: C.Kwssl2.MaxConn,
		}
		zgg.Servers["(KWSSL)"] = api

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})

}

var _ z.Server = (*Kwssl2Api)(nil)

type Kwssl2Api struct {
	Address string
	Routers map[string]string
	MaxConn int
	Error   error

	sem      chan z.Sem
	listener net.Listener
}

func (api *Kwssl2Api) RunServe(key string) {
	z.Logn("[_kwssl2_]: http server booting... linsten:", key, api.Address)
	api.listener, api.Error = net.Listen("tcp", api.Address)
	if api.Error != nil {
		z.Logn("[_kwssl2_]: http server listen failed: ", api.Error)
		return
	}
	if api.MaxConn <= 0 {
		api.MaxConn = 100
	}
	if api.sem == nil {
		api.sem = make(chan z.Sem, api.MaxConn)
	}
	// if api.buffpool == nil {
	// 	api.buffpool = z.NewBufferPool(0, 0)
	// }
	for {
		conn, err := api.listener.Accept()
		if err != nil {
			// 当 listener 被关闭时，退出循环
			if errors.Is(err, net.ErrClosed) {
				return // ne, ok := err.(*net.OpError); ok && ne.Err == net.ErrClosed
			}
			z.Logn("[_kwssl2_]: http server accept error: ", err)
			continue
		}
		select {
		case api.sem <- z.Sem{}:
			go api.handle(conn) // 获取一个信号量, 允许处理新的连接
		default:
			z.Logn("[_kwssl2_]: max connections reached, rejecting new connection")
			conn.Close() // 直接关闭连接，拒绝新的连接, 防止过多的连接占用资源
		}
	}
}

func (api *Kwssl2Api) Shutdown(ctx context.Context) error {
	if api.listener == nil {
		return nil
	}
	return api.listener.Close()
}

func (api *Kwssl2Api) handle(src net.Conn) {
	defer func() {
		src.Close()
		<-api.sem
	}()

	sni, buf, err := tlsx.PeekSNI(src)
	if err != nil {
		z.Logn("[_kwssl2_]: PeekSNI error: ", err)
		return
	}
	target, ok := api.Routers[sni]
	if !ok {
		z.Logn("[_kwssl2_]: unknown host: ", sni)
		return
	}
	if !strings.ContainsRune(target, ':') {
		target += ":443"
	}
	// 连接目标服务器
	dst, err := net.Dial("tcp", target)
	if err != nil {
		z.Logn("[_kwssl2_]: dial backend failed: ", err)
		return
	}
	defer dst.Close()
	// 将客户端发送的 ClientHello 消息转发到目标服务器, 透传
	if _, err := dst.Write(buf); err != nil {
		z.Logn("[_kwssl2_]: write client hello failed: ", err)
		return
	}

	done := make(chan z.Sem)
	go func() {
		defer close(done)
		io.Copy(dst, src)
	}()
	io.Copy(src, dst)
	<-done
}
