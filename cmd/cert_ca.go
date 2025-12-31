package cmd

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"os"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/ze/crt"
)

func init() {
	z.CMDR["cert-exp"] = CertEXP
	z.CMDR["cert-k8s"] = CertK8S
}

func CertEXP() {
	// /etc/kubernetes/pki
	// /var/lib/rancher/k3s/server/tls
	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	// caPath = "_out/_ca.crt"
	pemBts, err := os.ReadFile(caPath)
	if err != nil {
		z.Fatal(err)
	}
	pemBlk, _ := pem.Decode(pemBts)
	if pemBlk == nil {
		z.Fatal(err)
	}
	pemCrt, err := x509.ParseCertificate(pemBlk.Bytes)
	if err != nil {
		z.Fatal(err)
	}
	z.Println("expired: ", pemCrt.NotAfter.Format(time.RFC3339))
}

func CertK8S() {

	crtFile := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	keyFile := "_out/k3s.key"
	// crtFile = "_out/sa.crt"
	// keyFile = "_out/sa.key"

	crtBts, err := os.ReadFile(crtFile)
	if err != nil {
		z.Fatal(err)
	}
	keyBts, err := os.ReadFile(keyFile)
	if err != nil {
		z.Fatal(err)
	}

	crt, err := crt.CreateCE(nil, "", 0, []string{"api.exp.com"}, []net.IP{{127, 0, 0, 1}}, crtBts, keyBts)
	if err != nil {
		z.Fatal(err)
	}

	// ------------------------------------------------------------------------
	println("=================== cert .crt ===================")
	println(crt.Crt)
	println("=================== cert .key ===================")
	println(crt.Key)
	println("=================== cert .b64 ===================")
	println(base64.StdEncoding.EncodeToString([]byte(crt.Crt)))
	println("")
	println("=================================================")

	os.WriteFile("_out/sa.crt", []byte(crt.Crt), 0644)
	os.WriteFile("_out/sa.key", []byte(crt.Key), 0644)
}
