// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package tlsx_test

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/suisrc/zgg/z/ze/tlsx"
)

// go test -v ze/crt/cert_test.go -run Test_cert

func Test_cert(t *testing.T) {

	// curl --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt https://127.0.0.1:442/healthz
	// curl -s --cacert _out/cert/ca.crt https://127.0.0.1:442/healthz | jq

	// 读取 cert-ca 文件内容给 cert.CertConfig 对象
	// bts, _ := os.ReadFile("../../_out/cert-ca.json")
	cfg := tlsx.CertConfig{
		"ca": {
			Expiry: "18282d", // 50年
			SubjectName: tlsx.SignSubject{
				Organization:     "ca",
				OrganizationUnit: "ca",
			},
		},
		"sa": {
			Expiry: "10y",
			SubjectName: tlsx.SignSubject{
				Organization:     "sa",
				OrganizationUnit: "sa",
			},
		},
		"default": {
			Expiry: "10y",
			SubjectName: tlsx.SignSubject{
				Organization:     "default",
				OrganizationUnit: "default",
			},
		},
	}
	// json.Unmarshal(bts, cfg)

	// 生成证书
	ca, err := tlsx.CreateCA(cfg, "ca")
	if err != nil {
		panic(err)
	}
	sa, err := tlsx.CreateSA(cfg, "sa", []byte(ca.Crt), []byte(ca.Key))
	if err != nil {
		panic(err)
	}

	ct, err := tlsx.CreateCE(cfg, "dev1", []string{"dev1.com"}, []net.IP{{127, 0, 0, 1}}, []byte(sa.Crt), []byte(sa.Key))
	if err != nil {
		panic(err)
	}
	// assert.Nil(t, err)

	// os.Mkdir("../_out/cert", 0644)
	// 保存证书
	os.WriteFile("../../_out/cert/ca.crt", []byte(ca.Crt), 0644)
	os.WriteFile("../../_out/cert/ca.key", []byte(ca.Key), 0644)
	os.WriteFile("../../_out/cert/sa.crt", []byte(sa.Crt), 0644)
	os.WriteFile("../../_out/cert/sa.key", []byte(sa.Key), 0644)
	os.WriteFile("../../_out/cert/dev1.crt", []byte(ct.Crt+sa.Crt), 0644)
	os.WriteFile("../../_out/cert/dev1.key", []byte(ct.Key), 0644)

}

// go test -v ze/crt/cert_test.go -run Test_verify

// 验证服务器证书链
func Test_verify(t *testing.T) {
	// 加载根CA证书（信任锚）
	rootCertBytes, err := os.ReadFile("../../_out/cert/ca.crt")
	if err != nil {
		panic(err)
	}
	rootCertPool := x509.NewCertPool()
	rootCertPool.AppendCertsFromPEM(rootCertBytes)

	// 加载服务器证书和子CA证书（形成证书链）
	serverCertsBytes, err := os.ReadFile("../../_out/cert/dev1.crt")
	if err != nil {
		panic(err)
	}
	var serverCert *x509.Certificate
	subCertPool := x509.NewCertPool()
	count := 0
	certs := strings.Split(string(serverCertsBytes), "-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----")
	for _, cert := range certs {
		count++
		if count == 1 {
			data := cert + "-----END CERTIFICATE-----"
			block, _ := pem.Decode([]byte(data))
			serverCert, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				panic(err)
			}
			// println(data)
		} else if count == len(certs) {
			data := "-----BEGIN CERTIFICATE-----" + cert
			block, _ := pem.Decode([]byte(data))
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				panic(err)
			}
			subCertPool.AddCert(cert)
			// println(data)
		} else {
			block, _ := pem.Decode([]byte(cert + "-----END CERTIFICATE-----"))
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				panic(err)
			}
			subCertPool.AddCert(cert)
		}
	}
	println("===================", count)

	// 验证证书链
	opts := x509.VerifyOptions{
		Roots:         rootCertPool, // 信任的根CA
		Intermediates: subCertPool,  // 中间CA（子CA）
		DNSName:       "dev1.com",   // 验证的域名（需与服务器证书SAN匹配）
	}
	if _, err := serverCert.Verify(opts); err != nil {
		panic(err)
	}

	println("服务器证书验证通过！")
}

// go test -v ze/crt/cert_test.go -run Test_cer1

func Test_cer1(t *testing.T) {
	crt, err := tlsx.CreateCE(nil, "dev1", []string{"dev1.com"}, nil, nil, nil)
	if err != nil {
		panic(err)
	}
	// assert.Nil(t, err)

	os.WriteFile("../../_out/cert/dev2.crt", []byte(crt.Crt), 0644)
	os.WriteFile("../../_out/cert/dev2.key", []byte(crt.Key), 0644)

}
