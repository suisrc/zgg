// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package crt

import (
	"crypto/tls"
	"errors"
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

	_lock sync.Mutex
	_lmap map[string]*tls.Certificate
}

func (aa *TLSAutoConfig) GetCert(sni string, ipp string) (*tls.Certificate, error) {
	key := sni
	if key == "" {
		if ipp == "" {
			return nil, errors.New("sni and ipp is empty")
		}
		var err error
		key, _, err = net.SplitHostPort(ipp)
		if err != nil {
			if z.IsDebug() {
				zc.Println("[_tlsauto]: NewCertificate: ", ipp, " error: ", err)
			}
			return nil, err
		}
	}
	if aa._lmap != nil {
		if ct, ok := aa._lmap[key]; ok {
			return ct, nil
		}
	}
	// ----------------------------------------------------------------------------
	aa._lock.Lock()
	defer aa._lock.Unlock()
	if aa._lmap == nil {
		aa._lmap = make(map[string]*tls.Certificate)
	}
	if ct, ok := aa._lmap[key]; ok {
		return ct, nil
	}
	var err error
	var cer SignResult
	if sni != "" {
		cer, err = CreateCE(aa.CertConf, "", []string{sni}, nil, aa.CaCrtBts, aa.CaKeyBts)
	} else {
		sip := net.ParseIP(key)
		cer, err = CreateCE(aa.CertConf, "", nil, []net.IP{sip}, aa.CaCrtBts, aa.CaKeyBts)
	}
	if err != nil {
		if z.IsDebug() {
			zc.Println("[_tlsauto]: NewCertificate: ", key, " error: ", err)
		}
		return nil, err
	}
	if aa.IsSaCert {
		cer.Crt += string(aa.CaCrtBts)
	}
	if z.IsDebug() {
		zc.Println("[_tlsauto]: NewCertificate: ", key)
		zc.Printf("=============== cert .crt ===============%s\n%s\n", key, cer.Crt)
		zc.Printf("=============== cert .key ===============%s\n%s\n", key, cer.Key)
		zc.Println("=========================================")
	}
	ct, err := tls.X509KeyPair([]byte(cer.Crt), []byte(cer.Key))
	if err != nil {
		zc.Println("[_tlsauto]: NewCertificate: ", key, " load error: ", err)
		return nil, err
	}
	aa._lmap[key] = &ct
	return &ct, nil
}

func (aa *TLSAutoConfig) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return aa.GetCert(hello.ServerName, hello.Conn.LocalAddr().String())
}

func (aa *TLSAutoConfig) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if cert, err := aa.GetCert(hello.ServerName, hello.Conn.LocalAddr().String()); err != nil {
		return nil, err
	} else {
		return &tls.Config{Certificates: []tls.Certificate{*cert}}, nil
	}
}
