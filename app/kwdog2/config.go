package kwdog2

import (
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gte"
)

var (
	C = struct {
		Kwdog2 KwdogConfig
		Proxy2 ProxyConfig
		Kwssl2 KwsslConfig
	}{}

	AuthzDefaultFunc  = gte.NewAuthzF1kin
	RecordFrowardFunc = gte.ToRecord0
	RecordReverseFunc = gte.ToRecord1
)

func Init3(ifn1 InitKwdogFunc, ifn2 InitProxyFunc, ifn3 InitKwsslFunc) {
	z.Config(&C)
	InitKwdog(ifn1)
	InitProxy(ifn2)
	InitKwssl(ifn3)
}
