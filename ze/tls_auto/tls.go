// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package tlsauto

import (
	"crypto/tls"
	"flag"
	"net"
	"os"
	"sync"

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
	IsSub bool   `json:"casub"`
}

func init() {
	zc.Register(&C)
	flag.StringVar(&(C.Server.CrtCA), "cacrt", "", "http server crt ca file")
	flag.StringVar(&(C.Server.KeyCA), "cakey", "", "http server key ca file")
	flag.BoolVar(&C.Server.IsSub, "casub", false, "是否是中间证书")

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
		cfg.GetCertificate = (&TLSAutoConfig{
			CaKeyBts: caKeyBts,
			CaCrtBts: caCrtBts,
			CertConf: certConf,
			IsSubCa:  C.Server.IsSub,
		}).GetCertificate
		zgg.TLSConf = cfg

		return nil
	})
}

type TLSAutoConfig struct {
	CaKeyBts []byte
	CaCrtBts []byte
	CertConf crt.CertConfig
	IsSubCa  bool

	cache sync.Map // 缓存池
}

func (aa *TLSAutoConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if ct, ok := aa.cache.Load(hello.ServerName); ok {
		return ct.(*tls.Certificate), nil
	}

	var err error
	var sni string
	var cer crt.SignResult
	if hello.ServerName == "" {
		sni, _, err = net.SplitHostPort(hello.Conn.LocalAddr().String())
		if err == nil {
			sip := net.ParseIP(sni)
			cer, err = crt.CreateCE(aa.CertConf, "", nil, []net.IP{sip}, aa.CaCrtBts, aa.CaKeyBts)
		}
	} else {
		sni = hello.ServerName
		cer, err = crt.CreateCE(aa.CertConf, "", []string{sni}, nil, aa.CaCrtBts, aa.CaKeyBts)
	}
	if err != nil {
		if z.IsDebug() {
			zc.Println("[_tlsauto]: GetCertificate: ", sni, " error: ", err)
		}
		return nil, err
	}
	if aa.IsSubCa {
		cer.Crt += string(aa.CaCrtBts)
	}
	if z.IsDebug() {
		zc.Println("[_tlsauto]: GetCertificate: ", sni)
		zc.Printf("=================== cert .crt ===================%s\n%s\n", sni, cer.Crt)
		zc.Printf("=================== cert .key ===================%s\n%s\n", sni, cer.Key)
		zc.Println("=================================================")
	}
	ct, err := tls.X509KeyPair([]byte(cer.Crt), []byte(cer.Key))
	if err != nil {
		zc.Println("[_tlsauto]: GetCertificate: ", sni, " load error: ", err)
		return nil, err
	}
	aa.cache.Store(hello.ServerName, &ct)
	return &ct, nil
}

func (aa *TLSAutoConfig) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	cert, err := aa.GetCertificate(hello)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
	}, nil
}
