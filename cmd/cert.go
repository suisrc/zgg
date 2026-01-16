package cmd

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/tlsx"
)

func init() {
	z.CMD["cert"] = Cert        // 创建一个证书
	z.CMD["certca"] = CertCA    // 创建一个根证书
	z.CMD["certsa"] = CertSA    // 创建一个中间证书
	z.CMD["certce"] = CertCE    // 通过中间证书创建一个证书
	z.CMD["cert-exp"] = CertEXP // 验证生疏的过期时间
}

// -------------------------------------------------------------------
// -------------------------------------------------------------------
// -------------------------------------------------------------------

func Cert() {
	var (
		path   string
		cname  string
		domain string
	)
	flag.StringVar(&path, "path", "std", "cert folder path")
	flag.StringVar(&cname, "cname", "default", "cert common name")
	flag.StringVar(&domain, "domain", "localhost", "cert domain, use ',' to split")
	flag.Parse() // flag.CommandLine.Parse(os.Args[2:])
	if path == "" {
		println("cert path is empty")
		return
	}
	domains := strings.Split(domain, ",")
	if len(domains) == 0 {
		println("cert domain is empty")
		return
	}
	crt, err := tlsx.CreateCE(nil, cname, domains, nil, nil, nil)
	if err != nil {
		println(err.Error())
		return
	}

	// ------------------------------------------------------------------------
	if path == "std" {
		println("=================== cert .crt ===================")
		println(crt.Crt)
		println("=================== cert .key ===================")
		println(crt.Key)
		println("=================== cert .b64 ===================")
		println(base64.StdEncoding.EncodeToString([]byte(crt.Crt)))
		println("")
		println("=================================================")
		return
	}

	// ------------------------------------------------------------------------
	println("write files: " + filepath.Join(path, cname+".crt | key | b64(crt)"))
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, cname+".crt"), []byte(crt.Crt), 0644)
	os.WriteFile(filepath.Join(path, cname+".key"), []byte(crt.Key), 0644)

	b64 := base64.StdEncoding.EncodeToString([]byte(crt.Crt))
	os.WriteFile(filepath.Join(path, cname+".b64"), []byte(b64), 0644)
	println("================ cert create success ================")

}

// -------------------------------------------------------------------
// -------------------------------------------------------------------
// -------------------------------------------------------------------

func CertCA() {
	var (
		path  string
		cname string
	)
	flag.StringVar(&path, "path", "std", "cert folder path")
	flag.StringVar(&cname, "cname", "ca", "cert common name")
	flag.Parse() // flag.CommandLine.Parse(os.Args[2:])
	if path == "" {
		println("cert path is empty")
		return
	}
	crt, err := tlsx.CreateCA(nil, cname)
	if err != nil {
		z.Fatalln(err)
	}

	// ------------------------------------------------------------------------
	if path == "std" {
		println("=================== cert .crt ===================")
		println(crt.Crt)
		println("=================== cert .key ===================")
		println(crt.Key)
		println("=================== cert .b64 ===================")
		println(base64.StdEncoding.EncodeToString([]byte(crt.Crt)))
		println("")
		println("=================================================")
	}

	// ------------------------------------------------------------------------
	println("write files: " + filepath.Join(path, cname+".crt | key | b64(crt)"))
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, cname+".crt"), []byte(crt.Crt), 0644)
	os.WriteFile(filepath.Join(path, cname+".key"), []byte(crt.Key), 0644)

	b64 := base64.StdEncoding.EncodeToString([]byte(crt.Crt))
	os.WriteFile(filepath.Join(path, cname+".b64"), []byte(b64), 0644)
	println("================ cert create success ================")
}

// -------------------------------------------------------------------
// -------------------------------------------------------------------
// -------------------------------------------------------------------

func CertSA() {
	var (
		cacn  string
		path  string
		cname string
	)
	flag.StringVar(&path, "path", "", "cert folder path")
	flag.StringVar(&cacn, "cacn", "ca", "cert ca common name")
	flag.StringVar(&cname, "cname", "sa", "cert common name")
	flag.Parse() // flag.CommandLine.Parse(os.Args[2:])
	if path == "" {
		println("cert path is empty")
		return
	}

	crtCaBts, err := os.ReadFile(filepath.Join(path, cacn+".crt"))
	if err != nil {
		z.Fatalln(err)
	}
	keyCaBts, err := os.ReadFile(filepath.Join(path, cacn+".key"))
	if err != nil {
		z.Fatalln(err)
	}

	crt, err := tlsx.CreateSA(nil, cname, crtCaBts, keyCaBts)
	if err != nil {
		z.Fatalln(err)
	}
	// ------------------------------------------------------------------------
	println("write files: " + filepath.Join(path, cname+".crt | key | b64(crt)"))
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, cname+".crt"), []byte(crt.Crt), 0644)
	os.WriteFile(filepath.Join(path, cname+".key"), []byte(crt.Key), 0644)

	b64 := base64.StdEncoding.EncodeToString([]byte(crt.Crt))
	os.WriteFile(filepath.Join(path, cname+".b64"), []byte(b64), 0644)
	println("================ cert create success ================")
}

// -------------------------------------------------------------------
// -------------------------------------------------------------------
// -------------------------------------------------------------------

func CertCE() {
	var (
		sacn   string
		path   string
		cname  string
		domain string
	)
	flag.StringVar(&sacn, "cacn", "sa", "cert sa common name")
	flag.StringVar(&path, "path", "", "cert folder path")
	flag.StringVar(&cname, "cname", "default", "cert common name")
	flag.StringVar(&domain, "domain", "localhost", "cert domain, use ',' to split")
	flag.Parse() // flag.CommandLine.Parse(os.Args[2:])
	if path == "" {
		println("cert path is empty")
		return
	}
	domains := strings.Split(domain, ",")
	if len(domains) == 0 {
		println("cert domain is empty")
		return
	}

	crtSaBts, err := os.ReadFile(filepath.Join(path, sacn+".crt"))
	if err != nil {
		z.Fatalln(err)
	}
	keySaBts, err := os.ReadFile(filepath.Join(path, sacn+".key"))
	if err != nil {
		z.Fatalln(err)
	}

	crt, err := tlsx.CreateCE(nil, cname, domains, nil, crtSaBts, keySaBts)
	if err != nil {
		println(err.Error())
		return
	}

	// ------------------------------------------------------------------------
	println("write files: " + filepath.Join(path, cname+".crt | key | b64(crt)"))
	os.MkdirAll(path, 0755)
	os.WriteFile(filepath.Join(path, cname+".crt"), []byte(crt.Crt+string(crtSaBts)), 0644)
	os.WriteFile(filepath.Join(path, cname+".key"), []byte(crt.Key), 0644)

	b64 := base64.StdEncoding.EncodeToString([]byte(crt.Crt))
	os.WriteFile(filepath.Join(path, cname+".b64"), []byte(b64), 0644)
	println("================ cert create success ================")
}

// -------------------------------------------------------------------
// -------------------------------------------------------------------
// -------------------------------------------------------------------

func CertEXP() {
	// /etc/kubernetes/pki
	// /var/lib/rancher/k3s/server/tls
	var (
		path string
	)
	flag.StringVar(&path, "path", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "crt file path")
	pemBts, err := os.ReadFile(path)
	if err != nil {
		z.Fatalln(err)
	}
	pemBlk, _ := pem.Decode(pemBts)
	if pemBlk == nil {
		z.Fatalln(err)
	}
	pemCrt, err := x509.ParseCertificate(pemBlk.Bytes)
	if err != nil {
		z.Fatalln(err)
	}
	z.Println("expired: ", pemCrt.NotAfter.Format(time.RFC3339))
}
