package gtw

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/suisrc/zgg/ze/crt"
)

// 中间人攻击（MITM, Man-in-the-Middle）: 流量监控
type ForwardProxy struct {
	GatewayProxy
	TLSConfig *crt.TLSAutoConfig
}

func (p *ForwardProxy) ServeHTTP(rw http.ResponseWriter, rr *http.Request) {
	if rr.Method == http.MethodConnect {
		p.ServeHTTPS(rw, rr) // 处理HTTPS请求（CONNECT方法）
		return
	}
	p.GatewayProxy.ServeHTTP(rw, rr) // 处理HTTP请求（直接转发）
}

func (p *ForwardProxy) ServeHTTPS(rw http.ResponseWriter, rr *http.Request) {
	if p.TLSConfig == nil {
		p.Logf("[_forward]: tls config is nil")
		return
	}

	target := rr.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target += ":443"
	}
	host, _, _ := net.SplitHostPort(target)
	// 创建虚拟证书
	cert, err := p.TLSConfig.GetCert(host, "")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	// 升级连接为TLS（与客户端建立加密连接）
	hi, ok := rw.(http.Hijacker)
	if !ok {
		http.Error(rw, "non-hijacker ResponseWriter type", http.StatusInternalServerError)
		return
	}
	conn, _, err := hi.Hijack()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()
	// 向客户端发送200响应，确认CONNECT成功
	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	// 用自定义证书与客户端建立TLS连接
	tlsc := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	}
	ctls := tls.Server(conn, tlsc)
	if err := ctls.Handshake(); err != nil {
		p.Logf("[_forward]: tls handshake error: %s", err.Error())
		return
	}
	defer ctls.Close()

	// 将 ctls 转换为新的 http.ResponseWriter 和 *http.Request
	// 1. 从加密连接中读取客户端的HTTP请求（需用bufio包装以支持HTTP解析）
	tr, err := http.ReadRequest(bufio.NewReader(ctls))
	if err != nil {
		p.Logf("[_forward]: tls read client request error: %s", err.Error())
		return
	}
	tr.RemoteAddr = rr.RemoteAddr
	tr.URL.Scheme = "https"
	tr.URL.Host = tr.Host
	tw := NewTlsResponseWriter(ctls)
	// defer tw.(http.Flusher).Flush()
	p.GatewayProxy.ServeHTTP(tw, tr)
}

// ----------------------------------------------------------------------

func NewTlsResponseWriter(conn *tls.Conn) http.ResponseWriter {
	return &tlsResponseWriter{
		header: make(http.Header),
		status: http.StatusOK, // 默认状态码200
		writer: conn,
		// writer: bufio.NewWriter(conn),
	}
}

var _ http.ResponseWriter = (*tlsResponseWriter)(nil)

// 实现http.ResponseWriter接口
type tlsResponseWriter struct {
	header http.Header // 响应头部
	status int         // 响应状态码（默认200）
	wrote  bool        // 是否已写入响应（避免重复写入状态行）
	writer io.Writer
	// writer *bufio.Writer // 缓冲写入，提升性能， gateway中有缓存， 这里不需要缓存
}

// http.Flusher
// func (w *tlsResponseWriter) Flush() {
// 	w.writer.Flush()
// }

func (w *tlsResponseWriter) Header() http.Header {
	return w.header
}
func (w *tlsResponseWriter) WriteHeader(statusCode int) {
	if w.wrote {
		return // 已写入响应，忽略重复调用
	}
	w.status = statusCode
}

func (w *tlsResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		// 第一步：写入状态行（如HTTP/1.1 200 OK）
		proto := "HTTP/1.1"
		statusText := http.StatusText(w.status)
		if _, err := fmt.Fprintf(w.writer, "%s %d %s\r\n", proto, w.status, statusText); err != nil {
			return 0, err
		}
		// 第二步：写入响应头部
		if err := w.header.Write(w.writer); err != nil {
			return 0, err
		}
		// 第三步：写入空行（分隔头部与Body）
		if _, err := w.writer.Write([]byte{'\r', '\n'}); err != nil {
			return 0, err
		}
		w.wrote = true // 标记已写入响应头
	}
	// 第四步：写入Body内容
	return w.writer.Write(b)
}
