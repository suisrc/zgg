// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package tlsx

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/suisrc/zgg/z"
)

type TLSAutoConfig struct {
	CaKeyBts []byte
	CaCrtBts []byte
	CertConf CertConfig
	IsSaCert bool

	lock sync.Mutex
	lmap map[string]*tls.Certificate
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
				z.Logn("[_tlsauto]: NewCertificate: ", ipp, " error: ", err)
			}
			return nil, err
		}
	}
	if aa.lmap != nil {
		if ct, ok := aa.lmap[key]; ok {
			return ct, nil
		}
	}
	// ----------------------------------------------------------------------------
	aa.lock.Lock()
	defer aa.lock.Unlock()
	if aa.lmap == nil {
		aa.lmap = make(map[string]*tls.Certificate)
	}
	if ct, ok := aa.lmap[key]; ok {
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
			z.Logn("[_tlsauto]: NewCertificate: ", key, " error: ", err)
		}
		return nil, err
	}
	if aa.IsSaCert {
		cer.Crt += string(aa.CaCrtBts)
	}
	if z.IsDebug() {
		z.Logn("[_tlsauto]: NewCertificate: ", key)
		z.Logf("=============== cert .crt ===============%s\n%s\n", key, cer.Crt)
		z.Logf("=============== cert .key ===============%s\n%s\n", key, cer.Key)
		z.Logn("=========================================")
	}
	ct, err := tls.X509KeyPair([]byte(cer.Crt), []byte(cer.Key))
	if err != nil {
		z.Logn("[_tlsauto]: NewCertificate: ", key, " load error: ", err)
		return nil, err
	}
	aa.lmap[key] = &ct
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

func PeekSNI(conn net.Conn) (string, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	if header[0] != 0x16 { // TLS handshake record
		return "", nil, errors.New("not tls handshake")
	}
	length := int(header[3])<<8 | int(header[4])
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return "", nil, err
	}
	buf := append(header, body...)
	if len(body) < 42 {
		return "", buf, errors.New("client hello too short")
	}
	idx := 5 + 4 + 2 // record header + handshake header + version
	idx += 32        // random
	if idx+1 > len(buf) {
		return "", buf, errors.New("invalid client hello")
	}
	sessionIdLen := int(buf[idx])
	idx += 1 + sessionIdLen
	if idx+2 > len(buf) {
		return "", buf, errors.New("invalid client hello")
	}
	cipherSuiteLen := int(buf[idx])<<8 | int(buf[idx+1])
	idx += 2 + cipherSuiteLen
	if idx+1 > len(buf) {
		return "", buf, errors.New("invalid client hello")
	}
	compressionLen := int(buf[idx])
	idx += 1 + compressionLen
	if idx+2 > len(buf) {
		return "", buf, errors.New("invalid client hello")
	}
	extensionsLen := int(buf[idx])<<8 | int(buf[idx+1])
	idx += 2
	endExt := idx + extensionsLen
	for idx+4 <= endExt && idx+4 <= len(buf) {
		extType := int(buf[idx])<<8 | int(buf[idx+1])
		extLen := int(buf[idx+2])<<8 | int(buf[idx+3])
		idx += 4
		if extType == 0 { // SNI extension
			if idx+2 > len(buf) {
				break
			}
			sniListLen := int(buf[idx])<<8 | int(buf[idx+1])
			idx += 2
			endSNI := idx + sniListLen
			for idx+3 <= endSNI {
				nameType := buf[idx]
				nameLen := int(buf[idx+1])<<8 | int(buf[idx+2])
				idx += 3
				if nameType == 0 {
					if idx+nameLen > len(buf) {
						break
					}
					return string(buf[idx : idx+nameLen]), buf, nil
				}
				idx += nameLen
			}
			break
		}
		idx += extLen
	}
	return "", buf, errors.New("sni not found")
}
