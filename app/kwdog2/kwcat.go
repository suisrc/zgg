package kwdog2

// cat: 通常指家猫、野猫，也可作为动词表示"连接、串联", 它的存在就是连接 https 流量内容
// https passthrough 透传
// curl --resolve ipinfo.io:443:127.0.0.1 https://ipinfo.io

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

type KwcatConfig struct {
	Disabled bool              `json:"disabled"`
	AddrPort string            `json:"addr"`
	Routers  map[string]string `json:"routers"`               // 其他路由
	MaxConn  int               `json:"maxconn" default:"100"` // 最大并发数
}

// 初始化方法， 处理 hdl 的而外配置接口 443
type InitKwcatFunc func(hdl *KwcatHandler, zgg *z.Zgg)

func InitKwcat(ifn InitKwcatFunc) {

	flag.BoolVar(&C.Kwcat2.Disabled, "c2disabled", true, "是否禁用kwcat2")
	flag.StringVar(&C.Kwcat2.AddrPort, "c2addr", "0.0.0.0:443", "代理服务器地址和端口")
	flag.Var(z.NewStrMap(&C.Kwcat2.Routers, z.HM{}), "c2rmap", "其他服务转发")

	z.Register("13-kwcat2", func(zgg *z.Zgg) z.Closed {
		if C.Kwcat2.Disabled {
			z.Logn("[_kwcat2_]: disabled")
			return nil
		}
		hdl := &KwcatHandler{
			Address: C.Kwcat2.AddrPort,
			Routers: C.Kwcat2.Routers,
			MaxConn: C.Kwcat2.MaxConn,
		}
		zgg.Servers.Add(hdl)

		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})

}

var _ z.Server = (*KwcatHandler)(nil)

type KwcatHandler struct {
	Address string
	Routers map[string]string
	MaxConn int
	Error   error

	sem      chan z.Sem
	listener net.Listener
}

func (hdl *KwcatHandler) Name() string {
	return "(KWCAT)"
}

func (hdl *KwcatHandler) Addr() string {
	return hdl.Address
}

func (hdl *KwcatHandler) RunServe() {
	hdl.listener, hdl.Error = net.Listen("tcp", hdl.Address)
	if hdl.Error != nil {
		z.Logn("[_kwcat2_]: http server listen failed: ", hdl.Error)
		return
	}
	if hdl.MaxConn <= 0 {
		hdl.MaxConn = 100
	}
	if hdl.sem == nil {
		hdl.sem = make(chan z.Sem, hdl.MaxConn)
	}
	// if hdl.buffpool == nil {
	// 	hdl.buffpool = z.NewBufferPool(0, 0)
	// }
	for {
		conn, err := hdl.listener.Accept()
		if err != nil {
			// 当 listener 被关闭时，退出循环
			if errors.Is(err, net.ErrClosed) {
				return // ne, ok := err.(*net.OpError); ok && ne.Err == net.ErrClosed
			}
			z.Logn("[_kwcat2_]: http server accept error: ", err)
			continue
		}
		select {
		case hdl.sem <- z.Sem{}:
			go hdl.handle(conn) // 获取一个信号量, 允许处理新的连接
		default:
			z.Logn("[_kwcat2_]: max connections reached, rejecting new connection")
			conn.Close() // 直接关闭连接，拒绝新的连接, 防止过多的连接占用资源
		}
	}
}

func (hdl *KwcatHandler) Shutdown(ctx context.Context) error {
	if hdl.listener == nil {
		return nil
	}
	return hdl.listener.Close()
}

func (hdl *KwcatHandler) handle(src net.Conn) {
	defer func() {
		src.Close()
		<-hdl.sem
	}()

	sni, buf, err := tlsx.PeekSNI(src)
	if err != nil {
		z.Logn("[_kwcat2_]: PeekSNI error: ", err)
		return
	}
	target, ok := sni, true // 默认使用 SNI 作为目标地址, 直接透传
	if len(hdl.Routers) > 0 {
		// 如果指定路由，使用路由规则进行匹配
		target, ok = hdl.Routers[sni]
	}
	if !ok {
		z.Logn("[_kwcat2_]: unknown host: ", sni)
		return
	}
	if !strings.ContainsRune(target, ':') {
		target += ":443"
	}
	if z.IsDebug() {
		z.Logf("[_kwcat2_]: %s -> %s\n", sni, target)
	}
	// 连接目标服务器
	dst, err := net.Dial("tcp", target)
	if err != nil {
		z.Logn("[_kwcat2_]: dial backend failed: ", err)
		return
	}
	defer dst.Close()
	// 将客户端发送的 ClientHello 消息转发到目标服务器, 透传
	if _, err := dst.Write(buf); err != nil {
		z.Logn("[_kwcat2_]: write client hello failed: ", err)
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
