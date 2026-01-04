// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package tlsauto

import (
	"crypto/tls"
	"flag"
	"os"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
	"github.com/suisrc/zgg/ze/crt"
)

var (
	C = struct {
		Server ServerConfig
	}{}
)

type ServerConfig struct {
	CrtCA string `json:"cacrt"`
	KeyCA string `json:"cakey"`
	IsSAA bool   `json:"casaa"`
}

func init() {
	zc.Register(&C)
	flag.StringVar(&(C.Server.CrtCA), "cacrt", "", "http server crt ca file")
	flag.StringVar(&(C.Server.KeyCA), "cakey", "", "http server key ca file")
	flag.BoolVar(&C.Server.IsSAA, "casaa", false, "是否是中间证书")

	z.Register("10-tlsauto", func(zgg *z.Zgg) z.Closed {
		if C.Server.CrtCA == "" || C.Server.KeyCA == "" {
			zc.Println("[_tlsauto]: cacrt file or cakey file is empty")
			return nil
		}
		zc.Println("[_tlsauto]: crt=", C.Server.CrtCA, " key=", C.Server.KeyCA)

		caCrtBts, err := os.ReadFile(C.Server.CrtCA)
		if err != nil {
			zgg.ServeStop("[_tlsauto]: cacrt file error:", err.Error())
			return nil
		}
		caKeyBts, err := os.ReadFile(C.Server.KeyCA)
		if err != nil {
			zgg.ServeStop("[_tlsauto]: cakey file error:", err.Error())
			return nil
		}
		certConf := crt.CertConfig{
			"default": {
				Expiry:  "10y",
				KeySize: 2048,
				SubjectName: crt.SignSubject{
					Organization:     "default",
					OrganizationUnit: "default",
				},
			},
		}

		cfg := &tls.Config{}
		cfg.GetCertificate = (&crt.TLSAutoConfig{
			CaKeyBts: caKeyBts,
			CaCrtBts: caCrtBts,
			CertConf: certConf,
			IsSaCert: C.Server.IsSAA,
		}).GetCertificate
		zgg.TLSConf = cfg

		return nil
	})
}
