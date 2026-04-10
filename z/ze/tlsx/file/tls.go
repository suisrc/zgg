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
)

var (
	G = struct {
		Server ServerConfig
	}{}
)

type ServerConfig struct {
	CrtFile string `json:"crtfile"`
	KeyFile string `json:"keyfile"`
}

func init() {
	z.Config(&G)
	flag.StringVar(&(G.Server.CrtFile), "crt", "", "http server crt file")
	flag.StringVar(&(G.Server.KeyFile), "key", "", "http server key file")

	z.Register("10-tlsfile", func(zgg *z.Zgg) z.Closed {
		if G.Server.CrtFile == "" || G.Server.KeyFile == "" {
			z.Logn("[_tlsfile]: crtfile or keyfile is empty")
			return nil
		}
		z.Logn("[_tlsfile]: crt=", G.Server.CrtFile, " key=", G.Server.KeyFile)
		var err error

		cfg := &tls.Config{}
		cfg.Certificates = make([]tls.Certificate, 1)
		cfg.Certificates[0], err = tls.LoadX509KeyPair(G.Server.CrtFile, G.Server.KeyFile)
		if err != nil {
			zgg.ServeStop("[_tlsfile]: error:", err.Error())
			return nil
		}
		zgg.TLSConf = cfg

		return nil
	})
}
