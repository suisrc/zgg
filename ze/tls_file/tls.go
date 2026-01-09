// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 通过文件加载 http 服务的证书文件

package tlsfile

import (
	"crypto/tls"
	"flag"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var (
	C = struct {
		Server ServerConfig
	}{}
)

type ServerConfig struct {
	CrtFile string `json:"crtfile"`
	KeyFile string `json:"keyfile"`
}

func init() {
	zc.Register(&C)
	flag.StringVar(&(C.Server.CrtFile), "crt", "", "http server crt file")
	flag.StringVar(&(C.Server.KeyFile), "key", "", "http server key file")

	z.Register("10-tlsfile", func(zgg *z.Zgg) z.Closed {
		if C.Server.CrtFile == "" || C.Server.KeyFile == "" {
			z.Println("[_tlsfile]: crtfile or keyfile is empty")
			return nil
		}
		z.Println("[_tlsfile]: crt=", C.Server.CrtFile, " key=", C.Server.KeyFile)
		var err error

		cfg := &tls.Config{}
		cfg.Certificates = make([]tls.Certificate, 1)
		cfg.Certificates[0], err = tls.LoadX509KeyPair(C.Server.CrtFile, C.Server.KeyFile)
		if err != nil {
			zgg.ServeStop("[_tlsfile]: error:", err.Error())
			return nil
		}
		zgg.TLSConf = cfg

		return nil
	})
}
