package crt_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/suisrc/zgg/ze/crt"
)

func Test_cert(t *testing.T) {
	// 读取 cert-ca 文件内容给 cert.CertConfig 对象
	bts, _ := os.ReadFile("../../_out/cert-ca.json")
	cfg := &crt.CertConfig{}
	json.Unmarshal(bts, cfg)

	// 生成证书
	ca, err := crt.CreateCA(cfg)
	if err != nil {
		panic(err)
	}
	// assert.Nil(t, err)

	ct, err := crt.CreateCE(cfg, "dev1", "", 0, []string{"sso.dev1.com"}, nil, []byte(ca.Crt), []byte(ca.Key))
	if err != nil {
		panic(err)
	}
	// assert.Nil(t, err)

	// os.Mkdir("../_out/cert", 0644)
	// 保存证书
	os.WriteFile("../../_out/cert/ca.crt", []byte(ca.Crt), 0644)
	os.WriteFile("../../_out/cert/ca.key", []byte(ca.Key), 0644)
	os.WriteFile("../../_out/cert/dev1.crt", []byte(ct.Crt), 0644)
	os.WriteFile("../../_out/cert/dev1.key", []byte(ct.Key), 0644)

}

// go test -v app/cert_test.go -run Test_cer1

func Test_cer1(t *testing.T) {
	crt, err := crt.CreateCE(nil, "dev1", "", 0, []string{"sso.dev1.com"}, nil, nil, nil)
	if err != nil {
		panic(err)
	}
	// assert.Nil(t, err)

	os.WriteFile("../../_out/cert/dev2.crt", []byte(crt.Crt), 0644)
	os.WriteFile("../../_out/cert/dev2.key", []byte(crt.Key), 0644)

}
