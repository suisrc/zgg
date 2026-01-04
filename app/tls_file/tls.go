package tlsfile

import (
	"crypto/tls"
	"flag"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var (
	C = struct {
		Server ServerConfig
	}{}
)

type ServerConfig struct {
	CrtFile string `json:"crtfile"`
	KeyFile string `json:"keyfile"`
}

func init() {
	zc.Register(&C)
	flag.StringVar(&(C.Server.CrtFile), "crt", "", "http server cer file")
	flag.StringVar(&(C.Server.KeyFile), "key", "", "http server key file")

	z.Register("10-tlsfile", func(srv z.IServer) z.Closed {
		if C.Server.CrtFile == "" || C.Server.KeyFile == "" {
			z.Println("tlsfile: crtfile or keyfile is empty")
			return nil
		}
		z.Println("tlsfile: crt=", C.Server.CrtFile, " key=", C.Server.KeyFile)
		var err error

		zgg := srv.(*z.Zgg)
		cfg := &tls.Config{}

		cfg.Certificates = make([]tls.Certificate, 1)
		cfg.Certificates[0], err = tls.LoadX509KeyPair(C.Server.CrtFile, C.Server.KeyFile)
		if err != nil {
			z.Printf("TLS error: %s\n", err)
			srv.ServeStop()
			return nil
		}
		zgg.TLSConf = cfg

		return nil
	})
}
