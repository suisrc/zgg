package cmd

import (
	"encoding/base64"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/ze/crt"
)

func init() {
	z.CMDR["cert"] = CertCreate
}

func CertCreate() {
	var (
		path   string
		cname  string
		domain string
	)
	flag.StringVar(&path, "path", "std", "cert folder path")
	flag.StringVar(&cname, "cname", "default", "cert common name")
	flag.StringVar(&domain, "domain", "localhost", "cert domain, use ',' to split")
	flag.Parse()
	// flag.CommandLine.Parse(os.Args[2:])
	if path == "" {
		println("cert path is empty")
		return
	}
	domains := strings.Split(domain, ",")
	if len(domains) == 0 {
		println("cert domain is empty")
		return
	}
	crt, err := crt.CreateCE(nil, cname, "", 0, domains, nil, nil, nil)
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
