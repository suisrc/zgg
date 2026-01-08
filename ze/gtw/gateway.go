// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 反向代理网关

package gtw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

type IGateway interface {
	ServeHTTP(rw http.ResponseWriter, req *http.Request)
	GetProxyName() string
	Logf(format string, args ...any)
	GetErrorHandler() func(rw http.ResponseWriter, req *http.Request, err error)
}

type GatewayProxy struct {
	ReverseProxy
	RecordPool RecordPool // 请求追踪
	Authorizer Authorizer // 权限认证
}

func (p *GatewayProxy) GetProxyName() string {
	if p.ProxyName == "" {
		return "gateway-proxy"
	}
	return p.ProxyName
}

func (p *GatewayProxy) NewRecord() IRecord {
	if p.RecordPool == nil {
		return nil
	}
	return p.RecordPool.Get()
}

func (p *GatewayProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// g.ReverseProxy.ServeHTTP(rw, req)

	// ==== recordtrace ====>>>
	record := p.NewRecord()
	if record != nil {
		defer record.Recycle()
		record.LogRequest(req)
	}
	// ==== recordtrace ====<<<

	ctx := req.Context()
	if ctx.Done() != nil {
		// CloseNotifier predates context.Context, and has been
		// entirely superseded by it. If the request contains
		// a Context that carries a cancellation signal, don't
		// bother spinning up a goroutine to watch the CloseNotify
		// channel (if any).
		//
		// If the request Context has a nil Done channel (which
		// means it is either context.Background, or a custom
		// Context implementation with no cancellation signal),
		// then consult the CloseNotifier if available.
	} else if cn, ok := rw.(http.CloseNotifier); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		notifyChan := cn.CloseNotify()
		go func() {
			select {
			case <-notifyChan:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	outreq := req.Clone(ctx)
	if req.ContentLength == 0 {
		outreq.Body = nil // Issue 16036: nil Body for http.Transport retries
	}
	if outreq.Body != nil {
		// Reading from the request body after returning from a handler is not
		// allowed, and the RoundTrip goroutine that reads the Body can outlive
		// this handler. This can lead to a crash if the handler panics (see
		// Issue 46866). Although calling Close doesn't guarantee there isn't
		// any Read in flight after the handle returns, in practice it's safe to
		// read after closing it.
		defer outreq.Body.Close()
	}
	if outreq.Header == nil {
		outreq.Header = make(http.Header) // Issue 33142: historical behavior was to always allocate
	}

	// ==== authentication ====>>>
	if (p.Authorizer != nil) && !p.Authorizer.Authz(p, rw, outreq, record) {
		return // failed
	}
	// ==== authentication ====<<<

	if (p.Director != nil) == (p.Rewrite != nil) {
		err := errors.New("ReverseProxy must have exactly one of Director or Rewrite set")
		record.SetRespBody([]byte("###error gateway, " + err.Error()))
		p.GetErrorHandler()(rw, req, err)
		return
	}

	if p.Director != nil {
		p.Director(outreq)
		if outreq.Form != nil {
			outreq.URL.RawQuery = cleanQueryParams(outreq.URL.RawQuery)
		}
	}
	outreq.Close = false

	reqUpType := upgradeType(outreq.Header)
	if !IsPrint(reqUpType) {
		err := fmt.Errorf("client tried to switch to invalid protocol %q", reqUpType)
		record.SetRespBody([]byte("###error gateway, " + err.Error()))
		p.GetErrorHandler()(rw, req, err)
		return
	}
	removeHopByHopHeaders(outreq.Header)

	// Issue 21096: tell backend applications that care about trailer support
	// that we support trailers. (We do, but we don't go out of our way to
	// advertise that unless the incoming client request thought it was worth
	// mentioning.) Note that we look at req.Header, not outreq.Header, since
	// the latter has passed through removeHopByHopHeaders.
	if HeaderValuesContainsToken(req.Header["Te"], "trailers") {
		outreq.Header.Set("Te", "trailers")
	}

	// After stripping all the hop-by-hop connection headers above, add back any
	// necessary for protocol upgrades, such as for websockets.
	if reqUpType != "" {
		outreq.Header.Set("Connection", "Upgrade")
		outreq.Header.Set("Upgrade", reqUpType)
	}

	if p.Rewrite != nil {
		// Strip client-provided forwarding headers.
		// The Rewrite func may use SetXForwarded to set new values
		// for these or copy the previous values from the inbound request.
		outreq.Header.Del("Forwarded")
		outreq.Header.Del("X-Forwarded-For")
		outreq.Header.Del("X-Forwarded-Host")
		outreq.Header.Del("X-Forwarded-Proto")

		// Remove unparsable query parameters from the outbound request.
		outreq.URL.RawQuery = cleanQueryParams(outreq.URL.RawQuery)

		pr := &ProxyRequest{
			In:  req,
			Out: outreq,
		}
		p.Rewrite(pr)
		outreq = pr.Out
	} else {
		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			// If we aren't the first proxy retain prior
			// X-Forwarded-For information as a comma+space
			// separated list and fold multiple headers into one.
			prior, ok := outreq.Header["X-Forwarded-For"]
			omit := ok && prior == nil // Issue 38079: nil now means don't populate the header
			if len(prior) > 0 {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			if !omit {
				outreq.Header.Set("X-Forwarded-For", clientIP)
			}
		}
	}

	if _, ok := outreq.Header["User-Agent"]; !ok {
		// If the outbound request doesn't have a User-Agent header set,
		// don't send the default Go HTTP client User-Agent.
		outreq.Header.Set("User-Agent", "")
	}

	var (
		roundTripMutex sync.Mutex
		roundTripDone  bool
	)
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			roundTripMutex.Lock()
			defer roundTripMutex.Unlock()
			if roundTripDone {
				// If RoundTrip has returned, don't try to further modify
				// the ResponseWriter's header map.
				return nil
			}
			h := rw.Header()
			CopyHeader(h, http.Header(header))
			rw.WriteHeader(code)

			// Clear headers, it's not automatically done by ResponseWriter.WriteHeader() for 1xx responses
			clear(h)
			return nil
		},
		ConnectStart: func(network, addr string) {
			if record != nil {
				record.SetUpstream(addr) // record upstream address
			}
		},
	}
	outreq = outreq.WithContext(httptrace.WithClientTrace(outreq.Context(), trace))

	// ==== recordtrace ====>>>
	if record != nil {
		record.LogOutRequest(outreq)
	}
	// ==== recordtrace ====<<<

	transport := p.Transport
	if transport == nil {
		transport = TransportGtw0
	}
	res, err := transport.RoundTrip(outreq)
	roundTripMutex.Lock()
	roundTripDone = true
	roundTripMutex.Unlock()
	if err != nil {
		record.SetRespBody([]byte("###error gateway, " + err.Error()))
		p.GetErrorHandler()(rw, outreq, err)
		return
	}
	// ==== recordtrace ====>>>
	if record != nil {
		record.LogResponse(res)
	}
	// ==== recordtrace ====<<<
	// Deal with 101 Switching Protocols responses: (WebSocket, h2c, etc)
	if res.StatusCode == http.StatusSwitchingProtocols {
		if !p.modifyResponse(rw, res, outreq) {
			return
		}
		p.handleUpgradeResponse(rw, outreq, res)
		return
	}

	removeHopByHopHeaders(res.Header)

	if !p.modifyResponse(rw, res, outreq) {
		return
	}

	CopyHeader(rw.Header(), res.Header)

	// The "Trailer" header isn't included in the Transport's response,
	// at least for *http.Transport. Build it up from Trailer.
	announcedTrailers := len(res.Trailer)
	if announcedTrailers > 0 {
		trailerKeys := make([]string, 0, len(res.Trailer))
		for k := range res.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		rw.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	rw.WriteHeader(res.StatusCode)

	// err = p.copyResponse(rw, res.Body, p.flushInterval(res))
	err = p.CopyResponse2(rw, res.Body, p.flushInterval(res), record)
	if err != nil {
		defer res.Body.Close()
		// Since we're streaming the response, if we run into an error all we can do
		// is abort the request. Issue 23643: ReverseProxy should use ErrAbortHandler
		// on read error while copying body.
		if !shouldPanicOnCopyError(req) {
			p.Logf("suppressing panic for copyResponse error in test; copy error: %v", err)
			return
		}
		panic(http.ErrAbortHandler)
	}
	res.Body.Close() // close now, instead of defer, to populate res.Trailer

	if len(res.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		http.NewResponseController(rw).Flush()
	}

	if len(res.Trailer) == announcedTrailers {
		CopyHeader(rw.Header(), res.Trailer)
		return
	}

	for k, vv := range res.Trailer {
		k = http.TrailerPrefix + k
		for _, v := range vv {
			rw.Header().Add(k, v)
		}
	}
}

func (p *GatewayProxy) CopyResponse2(dst http.ResponseWriter, src io.Reader, flushInterval time.Duration, record IRecord) error {
	var w io.Writer = dst

	if flushInterval != 0 {
		mlw := &maxLatencyWriter{
			dst:     dst,
			flush:   http.NewResponseController(dst).Flush,
			latency: flushInterval,
		}
		defer mlw.stop()

		// set up initial timer so headers get flushed even if body writes are delayed
		mlw.flushPending = true
		mlw.t = time.AfterFunc(flushInterval, mlw.delayedFlush)

		w = mlw
	}

	var buf []byte
	var bak []byte
	if p.BufferPool != nil {
		buf = p.BufferPool.Get()
		defer p.BufferPool.Put(buf)
		// ==== recordtrace ====>>>
		if record != nil {
			bak = p.BufferPool.Get()
			defer p.BufferPool.Put(bak)
		}
		// ==== recordtrace ====<<<
	}
	bsz, err := p.CopyBuffer2(w, src, buf, bak)
	// ==== recordtrace ====>>>
	if record != nil {
		record.LogRespBody(bsz, err, bak)
	}
	// ==== recordtrace ====<<<
	return err
}

func (p *GatewayProxy) CopyBuffer2(dst io.Writer, src io.Reader, buf []byte, bak []byte) (int64, error) {
	if len(buf) == 0 {
		buf = make([]byte, 32*1024)
	}
	baklen := int64(cap(bak)) // 缓存的长度
	var written int64
	for {
		nr, rerr := src.Read(buf)
		if rerr != nil && rerr != io.EOF && rerr != context.Canceled {
			p.Logf("copy buffer, read error during body copy: %v", rerr)
		}
		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				current := int(written)
				written += int64(nw)
				if written <= baklen {
					// bak = append(bak, buf[:nw]...)
					copy(bak[current:written], buf[:nw])
				} else {
					bak = bak[:0] // 无法存储，清理缓存
				}
			}
			if werr != nil {
				return written, werr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				rerr = nil
			}
			return written, rerr
		}
	}
}
