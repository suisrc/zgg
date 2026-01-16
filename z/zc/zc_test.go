// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package zc_test

import (
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/suisrc/zgg/z/zc"
)

// ZGG_S_A_B_D_0_A=12 go test -v z/zc/zc_test.go -run Test_config
// ZGG_S_C_A_E_2_N=12 go test -v z/zc/zc_test.go -run Test_config
// ZGG_DEBUG=1 go test -v z/zc/zc_test.go -run Test_config

func load() {
	println("init load")
}

func Test_config(t *testing.T) {
	zc.Register(&Data{})
	zc.Register(func() { println("init other") })
	zc.Register(load)
	log.Println("===========================loading")
	zc.LoadConfig("zc_test.toml,zc_test1.toml")
	log.Println("===========================loading")
}

// MiddleConfig 中间件启动和关闭
type Data struct {
	Debug   bool `default:"false"`
	Logger  bool `default:"true"`
	RunMode string
	B       struct {
		A *struct {
			B struct {
				D []struct {
					C int `default:"5" json:"a"`
				}
				E []*struct {
					C int `default:"6"`
				}
			}
		}
		B map[string]*struct {
			E []struct {
				C int `default:"5" json:"n"`
			}
		}
		C map[string]struct {
			E []struct {
				C int `default:"5" json:"n"`
			}
		}
	} `json:"s"`
}

// go test -v z/zc/zc_test.go -run Test_toml

func Test_toml(t *testing.T) {
	text, _ := os.ReadFile("zc_test.toml")
	data := zc.NewTOML(text).Map()
	log.Println(zc.ToStr2(data))
}

// go test -v z/zc/zc_test.go -run Test_toml2

func Test_toml2(t *testing.T) {
	data := &Data{}
	text, _ := os.ReadFile("zc_test.toml")
	zc.NewTOML(text).Decode(data, "json")
	log.Println(zc.ToStr2(data))
	// log.Println("===========================")
	// smap := zc.ToMap(data, "json", true)
	// log.Println(zc.ToStr2(smap))
}

// go test -v z/zc/zc_test.go -run Test_tags

func Test_tags(t *testing.T) {
	data := &Data{}
	text, _ := os.ReadFile("zc_test.toml")
	zc.NewTOML(text).Decode(data, "json")
	// tags, _, _ := zc.ToTagMap(data, "json", true, nil)
	tags := zc.ToTagVal(data, "json")
	log.Println("===========================")
	log.Println(zc.ToStr2(tags))
	log.Println("===========================")
	tags[len(tags)-1].Value.Set(reflect.ValueOf(12))
	tags[0].Value.Set(reflect.ValueOf(true))
	log.Println(zc.ToStr2(data))
}

// go test -v z/zc/zc_test.go -run Test_StrToArr

func Test_StrToArr(t *testing.T) {
	str := `["a","b"]`
	arr := zc.ToStrArr(str)
	log.Println(arr)

	str = `[1, 2]`
	arr = zc.ToStrArr(str)
	log.Println(arr)

	str = `[a, b]`
	arr = zc.ToStrArr(str)
	log.Println(arr)

	str = `['a', 'b']`
	arr = zc.ToStrArr(str)
	log.Println(arr)

	str = `123.123`
	aaa := zc.ToStrOrArr(str)
	log.Println(aaa)

	str = `"123.123"`
	aaa = zc.ToStrOrArr(str)
	log.Println(aaa)
	log.Println("======================")
	str = `["x", 1, true, 'y', "a'b", 'c"d', "e\\f"]`
	arr = zc.ToStrArr(str)
	for _, v := range arr {
		log.Println(v)
	}
	log.Println("======================")
	str = `["x", 1, true, 'y', "a'b", 'c"d', "e\\f"]`
	aaa = zc.ToStrOrArr(str)
	log.Println(aaa)

	log.Println("======================")
	str = `["x", "a'b", "e\\f"]`
	aaa = zc.ToStrOrArr(str)
	log.Println(aaa)

	log.Println("======================")

}

func Test_arr(t *testing.T) {
	arr := []string{"a", "b", "c"}
	log.Println(arr)
	// log.Println(arr[:-1])
}

// go test -v z/zc/zc_test.go -run Test_ToBasicVal

func Test_ToBasicVal(t *testing.T) {
	str := []string{"123", "456"}
	var val any
	var err error
	val, err = zc.ToBasicValue(reflect.TypeOf(float32(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(float64(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(int(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(int8(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(int32(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(int64(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf(""), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf([]byte("")), str)
	log.Printf("=======%#v | %v\n", string(val.([]byte)), err)
	val, err = zc.ToBasicValue(reflect.TypeOf(str), str)
	log.Printf("=======%#v | %v\n", val, err)

	val, err = zc.ToBasicValue(reflect.TypeOf([]int{}), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf([]int32{}), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = zc.ToBasicValue(reflect.TypeOf([][]byte{}), str)
	log.Printf("=======%#v | %v\n", string(val.([][]byte)[0]), err)

}

type RecordDat0 struct {
	Data0 string
}

type RecordDat1 struct {
	Data1 string
	RecordDat0
}

type Record struct {
	RecordDat1
	NameKey string
	AgeKey  int

	DataKey RecordData
	DataKe2 *RecordData
}

func (r Record) MarshalJSON() ([]byte, error) {
	return zc.ToJsonBytes(&r, "json", zc.LowerFirst, false)
}

type RecordData struct {
	NameKey string
	AgeKey  int
}

func (r RecordData) MarshalJSON() ([]byte, error) {
	return zc.ToJsonBytes(&r, "json", zc.Camel2Case, false)
}

// go test -v z/zc/zc_test.go -run Test_ToJson

func Test_ToJson(t *testing.T) {
	record := Record{
		NameKey: "x",
		AgeKey:  12,
		DataKey: RecordData{
			NameKey: "",
			AgeKey:  13,
		},
	}
	log.Println(zc.ToStr2(record))
}
