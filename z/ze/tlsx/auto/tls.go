// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 根据指定的 CA 动态生成证书

package tlsauto

import (
	"crypto/tls"
	"flag"
	"os"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/tlsx"
)

var (
	G = struct {
		Server ServerConfig
	}{}
)

type ServerConfig struct {
	CrtCA string `json:"cacrt"`
	KeyCA string `json:"cakey"`
	IsSAA bool   `json:"casaa"`
}

func init() {
	z.Config(&G)
	flag.StringVar(&(G.Server.CrtCA), "cacrt", "", "http server crt ca file")
	flag.StringVar(&(G.Server.KeyCA), "cakey", "", "http server key ca file")
	flag.BoolVar(&G.Server.IsSAA, "casaa", false, "是否是中间证书")

	z.Register("10-tlsauto", func(zgg *z.Zgg) z.Closed {
		if G.Server.CrtCA == "" || G.Server.KeyCA == "" {
			z.Logn("[_tlsauto]: cacrt file or cakey file is empty")
			return nil
		}
		z.Logn("[_tlsauto]: crt=", G.Server.CrtCA, " key=", G.Server.KeyCA)

		caCrtBts, err := os.ReadFile(G.Server.CrtCA)
		if err != nil {
			zgg.ServeStop("[_tlsauto]: cacrt file error:", err.Error())
			return nil
		}
		caKeyBts, err := os.ReadFile(G.Server.KeyCA)
		if err != nil {
			zgg.ServeStop("[_tlsauto]: cakey file error:", err.Error())
			return nil
		}
		certConf := tlsx.CertConfig{
			"default": {
				Expiry:  "10y",
				KeySize: 2048,
				SubjectName: tlsx.SignSubject{
					Organization:     "default",
					OrganizationUnit: "default",
				},
			},
		}

		cfg := &tls.Config{}
		cfg.GetCertificate = (&tlsx.TLSAutoConfig{
			CaKeyBts: caKeyBts,
			CaCrtBts: caCrtBts,
			CertConf: certConf,
			IsSaCert: G.Server.IsSAA,
		}).GetCertificate
		zgg.TLSConf = cfg

		return nil
	})
}
