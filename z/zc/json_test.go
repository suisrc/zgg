package zc_test

import (
	"testing"

	"github.com/suisrc/zgg/z/zc"
)

// go test -v z/zc/json_test.go -run Test_map

func Test_map(t *testing.T) {
	cac := map[string]any{
		"a": 1,
		"b": 2,
		"c": map[string]any{
			"d": 3,
			"e": 4,
			"f": []any{
				5,
				6,
			},
		},
	}

	value := zc.MapVal(cac, "c.d", "hello")
	t.Log(value)
	t.Log(zc.ToStr(cac))

	value = zc.MapKey(cac, "c.f.-1")
	t.Log(value)

	zc.MapVal(cac, "c.f.-0", "world")
	t.Log(zc.ToStr(cac))

	zc.MapVal(cac, "c.f.0", "123456")
	t.Log(zc.ToStr(cac))

	zc.MapVal(cac, "c.f.0", nil)
	t.Log(zc.ToStr(cac))

	str := zc.MapDef(cac, "c.f.1", "default")
	t.Log(str)
}
