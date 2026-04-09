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
		Kwbee2 KwbeeConfig
	}{}

	AuthzDefaultFunc  = gte.NewAuthzF1kin
	RecordFrowardFunc = gte.ToRecord0
	RecordReverseFunc = gte.ToRecord1
)

func Init3(ifn1 InitKwdogFunc) {
	z.Config(&C)
	InitKwdog(ifn1)
	InitKwrat(nil)
	InitKwcat(nil)
	InitKwbee(nil)
}
