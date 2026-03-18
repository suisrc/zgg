package zc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/suisrc/zgg/z/zc"
)

// go test -v z/zc/json_test.go -run Test_slice1

func Test_slice1(t *testing.T) {
	arrs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	arrs = zc.SliceDelete(arrs, 9)
	println(zc.ToStr(arrs))
	arrs = zc.SliceDelete(arrs, 0)
	println(zc.ToStr(arrs))
	arrs = zc.SliceDelete(arrs, 1)
	println(zc.ToStr(arrs))
	arrs = zc.SliceDelete(arrs, 3)
	println(zc.ToStr(arrs))
}

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
	zc.MapNew(dmap, "a.b.x.y.-0.-0.z", "123")
	t.Log("=================== ", zc.MapAny(dmap, "a.b.x.y.0.0.z", 0))
	zc.MapNew(dmap, "a.b.x.y.-0.-0.z", "123")
	zc.MapNew(dmap, "a.b.x.y.1.-0.z", "789")
	zc.MapNew(dmap, "a.b.x.y.1.-0.z", "567")
	zc.MapNew(dmap, "a.b.x.y.-1.-0.z", "234")
	t.Log("=================== ", zc.MapGet(dmap, "a.b.[.j=^*.2].j"))
	t.Log("=================== ", zc.MapGet(dmap, "a.b.[.j=>122].j"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=123]"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.='123']"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=int.123]"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=i64.123]"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=f64.123]"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=f32.123]"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.[.=i32.123]"))
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
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*"))
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.[.j=^*.2]"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.[.j=^*.2]"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.[.j=^3]"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.[.j=^3]"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.[.j=^1].j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.[.j=^1].j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.?.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a", "b", "?", "j"))
	t.Log("=================== ", zc.MapVars(dmap, "a", "b", "?", "j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a", "b", "*", "j"))
	t.Log("=================== ", zc.MapVars(dmap, "a", "b", "*", "j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.*.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.*.?.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.*.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.?.*.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.?.*.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.?.?.j"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.?.?.j"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.?.?.?"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.?.?.?"))
	t.Log("=================== ")
	t.Log("=================== ", zc.MapVals(dmap, "a.b.?.?.?.?"))
	t.Log("=================== ", zc.MapVars(dmap, "a.b.?.?.?.?"))
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
		zc.MapVals(dmap, "a.b.*.*.j")
		zc.MapVals(dmap, "a.b.*.?.j")
		zc.MapVals(dmap, "a.b.?.*.j")
	}
	t.Log("=================== ", time.Now().UnixMilli()-now.UnixMilli())
	now = time.Now()
	for range 1_000_000 {
		zc.MapVars(dmap, "a.b.*.*.j")
		zc.MapVars(dmap, "a.b.*.?.j")
		zc.MapVars(dmap, "a.b.?.*.j")
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
	t.Log("=================== ", zc.MapVals(jsonv, "spec.rules.*.http.paths.*.backend.service[.name=^fnt-].name"))
	t.Log("=================== ", zc.MapVals(jsonv, "spec.rules.*.http.paths.*.backend.service.name[.=^fnt-]"))
}

// go test -v z/zc/json_test.go -run Test_map5

func Test_map5(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{
					"q": map[string]any{
						"j": 123.456,
					},
				},
			},
		},
	}

	t.Log("=================== ", zc.MapSet(dmap, "a.b.-0", map[string]any{}))
	t.Log(zc.ToStr2(dmap))
}

// go test -v z/zc/json_test.go -run Test_map6

func Test_map6(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{
					"q": map[string]any{
						"j": 123.456,
					},
				},
			},
		},
	}

	t.Log("=================== ", zc.MapSet1(dmap, map[string]any{}, "a.b.-0"))
	t.Log("=================== ", zc.MapSet1(dmap, 123456, "a.b.0.q.j"))
	t.Log("=================== ", zc.MapSet1(dmap, 123456, "a.b.0.q.s"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 456789, "a.b.0.q.*"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 345678, "a.b.0.q.?"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 456789, "a.b.0.q.v.s.v"))
	t.Log("=================== ", zc.MapSets(dmap, 456789, "a.b.0.v.-0.s"))
	t.Log("=================== ", zc.MapSets(dmap, 123456, "a.b.0.v.-0.s"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 123456, "a.b.0.w.-0.s"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 123456, "a.b.0.w.-0.s"))
	t.Log(zc.ToStr2(dmap))
}

// go test -v z/zc/json_test.go -run Test_map7

func Test_map7(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{
					"q": map[string]any{
						"j": 123.456,
					},
				},
			},
		},
	}

	t.Log("=================== ", zc.MapSet1(dmap, map[string]any{"x": 456}, "a.b.-0"))
	t.Log("=================== ", zc.MapGet1(dmap, "a.b.0.q.j"))
	t.Log("=================== ", zc.MapGets(dmap, "a.b.*"))
	t.Log("=================== ", zc.MapGets(dmap, "a.b.?"))
	t.Log("=================== ", zc.MapGets(dmap, "a.b.1"))
	t.Log("=================== ", zc.MapSet1(dmap, 123567, "a.b.*"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapSets(dmap, 123, "a.b.*"))
	t.Log(zc.ToStr2(dmap))
}

// go test -v z/zc/json_test.go -run Test_map8

func Test_map8(t *testing.T) {
	dmap := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{
					"q": map[string]any{
						"j": 123.456,
					},
				},
			},
		},
	}
	t.Log("=================== ", zc.MapSets(dmap, nil, "a.b.0.w.-0.s"))
	t.Log(zc.ToStr2(dmap))
	t.Log("=================== ", zc.MapNew(dmap, "a.b.0.w.-0.s", nil))
	t.Log(zc.ToStr2(dmap))
}
