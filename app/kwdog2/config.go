package kwdog2

import (
	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/ze/gte"
)

var (
	C = struct {
		Kwdog2 KwdogConfig
		Kwrat2 KwratConfig
		Kwcat2 KwcatConfig
	}{}

	AuthzDefaultFunc  = gte.NewAuthzF1kin
	RecordFrowardFunc = gte.ToRecord0
	RecordReverseFunc = gte.ToRecord1
)

func Init3(ifn1 InitKwdogFunc, ifn2 InitKwratFunc, ifn3 InitKwcatFunc) {
	z.Config(&C)
	InitKwdog(ifn1)
	InitKwrat(ifn2)
	InitKwcat(ifn3)
}
