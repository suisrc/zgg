package kwdog2

import (
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gte"
)

var (
	C = struct {
		Kwdog2 KwdogConfig
		Proxy2 ProxyConfig
	}{}

	RecordFunc = gte.ToRecord0
	AuthzcFunc = gte.NewAuthzF1kin
)

func Init3(ifn1 InitializKwdogFunc, ifn2 InitializProxyFunc) {
	z.Config(&C)
	InitKwdog(ifn1)
	InitProxy(ifn2)
}
