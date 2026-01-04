// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package crt

import (
	"crypto/tls"
	"net"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

type TLSAutoConfig struct {
	CaKeyBts []byte
	CaCrtBts []byte
	CertConf CertConfig
	IsSaCert bool

	cache sync.Map // 缓存池
}

func (aa *TLSAutoConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if ct, ok := aa.cache.Load(hello.ServerName); ok {
		return ct.(*tls.Certificate), nil
	}

	var err error
	var sni string
	var cer SignResult
	if hello.ServerName == "" {
		sni, _, err = net.SplitHostPort(hello.Conn.LocalAddr().String())
		if err == nil {
			sip := net.ParseIP(sni)
			cer, err = CreateCE(aa.CertConf, "", nil, []net.IP{sip}, aa.CaCrtBts, aa.CaKeyBts)
		}
	} else {
		sni = hello.ServerName
		cer, err = CreateCE(aa.CertConf, "", []string{sni}, nil, aa.CaCrtBts, aa.CaKeyBts)
	}
	if err != nil {
		if z.IsDebug() {
			zc.Println("[_tlsconf]: GetCertificate: ", sni, " error: ", err)
		}
		return nil, err
	}
	if aa.IsSaCert {
		cer.Crt += string(aa.CaCrtBts)
	}
	if z.IsDebug() {
		zc.Println("[_tlsconf]: GetCertificate: ", sni)
		zc.Printf("=================== cert .crt ===================%s\n%s\n", sni, cer.Crt)
		zc.Printf("=================== cert .key ===================%s\n%s\n", sni, cer.Key)
		zc.Println("=================================================")
	}
	ct, err := tls.X509KeyPair([]byte(cer.Crt), []byte(cer.Key))
	if err != nil {
		zc.Println("[_tlsconf]: GetCertificate: ", sni, " load error: ", err)
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
	return &tls.Config{Certificates: []tls.Certificate{*cert}}, nil
}
