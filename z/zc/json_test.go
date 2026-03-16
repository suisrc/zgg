package zc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/suisrc/zgg/z/zc"
)

// go test -v z/zc/json_test.go -run Test_map1

func Test_map1(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "d",
				"d": 123,
				"j": "123",
				"k": int64(123),
				"l": float64(123),
				"m": float32(123),
				"n": int32(123),
				"e": "456",
				"f": 456.789,
				"g": uint16(0),
				"i": "N",
				"a": map[string]any{
					"j": "123",
				},
				"b": map[string]any{
					"j": "321",
				},
				"o": map[string]any{
					"j": 123,
				},
				"p": map[string]any{
					"j": true,
				},
				"q": map[string]any{
					"j": 123.456,
				},
			},
		},
	}

	t.Log("=================== ", zc.MapAny(dmap, "a.b.e", false))
	t.Log("=================== ", zc.MapAny(dmap, "a.b.f", false))
	t.Log("=================== ", zc.MapAny(dmap, "a.b.g", true))
	t.Log("=================== ", zc.MapAny(dmap, "a.b.h", false))
	t.Log("=================== ", zc.MapAny(dmap, "a.b.i", true))
	zc.MapNew(dmap, "a.b.x.y.-0.v", "123")
	t.Log("=================== ", zc.MapAny(dmap, "a.b.x.y.0.v", 0))
	zc.MapNew(dmap, "a.b.x.y.0.v", "456")
	t.Log("=================== ", zc.MapAny(dmap, "a.b.x.y.0.v", 0))
	// zc.MapVaz(dmap, "a.b.x.y.0", nil)
	zc.MapNew(dmap, "a.b.x.y.-0.-0.z", "123")
	t.Log("=================== ", zc.MapAny(dmap, "a.b.x.y.0.0.z", 0))
	zc.MapNew(dmap, "a.b.x.y.-0.-0.z", "123")
	zc.MapNew(dmap, "a.b.x.y.1.-0.z", "789")
	zc.MapNew(dmap, "a.b.x.y.1.-0.z", "567")
	zc.MapNew(dmap, "a.b.x.y.-1.-0.z", "234")
	t.Log("=================== ", zc.MapGet(dmap, "a.b.[.j=^*.2].j"))
	t.Log("=================== ", zc.MapGet(dmap, "a.b.[.j=>122].j"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=123]"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.='123']"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=int.123]"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=i64.123]"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=f64.123]"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=f32.123]"))
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.[.=i32.123]"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapSet(dmap, "a.b.d", nil))
	t.Log("=================== ", zc.MapAny(dmap, "a.b.x.y.1.[.z=^*.6].z", 0))
}

// go test -v z/zc/json_test.go -run Test_map2

func Test_map2(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "d",
				"d": 123,
				"e": "456",
				"f": 456.789,
				"g": uint16(0),
				"i": "N",
				"a": map[string]any{
					"j": "123",
				},
				"b": map[string]any{
					"j": "321",
				},
				"x": map[string]any{
					"a": map[string]any{
						"j": "123",
					},
					"b": map[string]any{
						"j": "123",
					},
				},
				"z": map[string]any{
					"a": map[string]any{
						"j": "123",
					},
					"b": map[string]any{
						"j": "123",
					},
				},
			},
		},
	}
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.[.j=^*.2]"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.[.j=^*.2]"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.[.j=^3]"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.[.j=^3]"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.[.j=^1].j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.[.j=^1].j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.?.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyRec(dmap, "a", "b", "?", "j"))
	t.Log("=================== ", zc.MapKeyItr(dmap, "a", "b", "?", "j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyRec(dmap, "a", "b", "*", "j"))
	t.Log("=================== ", zc.MapKeyItr(dmap, "a", "b", "*", "j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.*.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.*.?.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.*.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.?.*.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.?.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.?.?.j"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.?.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.?.?.?"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.?.?.?"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapKeyVal(dmap, "a.b.?.?.?.?"))
	t.Log("=================== ", zc.MapKeyVar(dmap, "a.b.?.?.?.?"))
}

// go test -v z/zc/json_test.go -run Test_map3

func Test_map3(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "d",
				"d": 123,
				"e": "456",
				"f": 456.789,
				"g": uint16(0),
				"i": "N",
				"a": map[string]any{
					"j": "123",
				},
				"b": map[string]any{
					"j": "321",
				},
				"x": map[string]any{
					"a": map[string]any{
						"j": "123",
					},
					"b": map[string]any{
						"j": "123",
					},
				},
				"z": map[string]any{
					"a": map[string]any{
						"j": "123",
					},
					"b": map[string]any{
						"j": "123",
					},
				},
			},
		},
	}
	now := time.Now()
	for range 1_000_000 {
		zc.MapKeyVar(dmap, "a.b.*.*.j")
		zc.MapKeyVar(dmap, "a.b.*.?.j")
		zc.MapKeyVar(dmap, "a.b.?.*.j")
	}
	t.Log("=================== ", time.Now().UnixMilli()-now.UnixMilli())
	now = time.Now()
	for range 1_000_000 {
		zc.MapKeyVal(dmap, "a.b.*.*.j")
		zc.MapKeyVal(dmap, "a.b.*.?.j")
		zc.MapKeyVal(dmap, "a.b.?.*.j")
	}
	t.Log("=================== ", time.Now().UnixMilli()-now.UnixMilli())
}

// go test -v z/zc/json_test.go -run Test_map4

func Test_map4(t *testing.T) {
	jsons := `
  {
    "apiVersion": "networking.k8s.io/v1",
    "kind": "Ingress",
    "metadata": {
      "name": "account-irs",
      "namespace": "rs-iam"
    },
    "spec": {
      "rules": [
        {
          "host": "",
          "http": {
            "paths": [
              {
                "backend": {
                  "service": {
                    "name": "end-fmes-adv-svc",
                    "port": {
                      "name": "http"
                    }
                  }
                },
                "path": "/api/adv",
                "pathType": "Prefix"
              },
              {
                "backend": {
                  "service": {
                    "name": "fnt-iam-account-m1-svc",
                    "port": {
                      "name": "http"
                    }
                  }
                },
                "path": "/m1/",
                "pathType": "Prefix"
              },
              {
                "backend": {
                  "service": {
                    "name": "fnt-account-svc",
                    "port": {
                      "name": "http"
                    }
                  }
                },
                "path": "/",
                "pathType": "Prefix"
              }
            ]
          }
        }
      ]
    }
  }`
	jsonv := map[string]any{}
	json.Unmarshal([]byte(jsons), &jsonv)
	// t.Log("=================== ", jsonv)
	t.Log("=================== ", zc.MapKeyVal(jsonv, "spec.rules.*.http.paths.*.backend.service[.name=^fnt-].name"))
	t.Log("=================== ", zc.MapKeyVal(jsonv, "spec.rules.*.http.paths.*.backend.service.name[.=^fnt-]"))
}
