package cfg_test

import (
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/suisrc/zgg/z/cfg"
)

// ZGG_S_A_B_D_0_A=12 go test -v z/cfg/zcfg_test.go -run Test_config
// ZGG_S_C_A_E_2_N=12 go test -v z/cfg/zcfg_test.go -run Test_config
// ZGG_DEBUG=1 go test -v z/cfg/zcfg_test.go -run Test_config

func Test_config(t *testing.T) {
	cfg.Register(&Data{})
	log.Println("===========================loading")
	cfg.MustLoad("zcfg_test.toml")
	cfg.PrintConfig()
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

// go test -v z/cfg/zcfg_test.go -run Test_toml

func Test_toml(t *testing.T) {
	text, _ := os.ReadFile("zcfg_test.toml")
	data := cfg.NewTOML(text).Map()
	log.Println(cfg.ToStr2(data))
}

// go test -v z/cfg/zcfg_test.go -run Test_toml2

func Test_toml2(t *testing.T) {
	data := &Data{}
	text, _ := os.ReadFile("zcfg_test.toml")
	cfg.NewTOML(text).Decode(data, "json")
	log.Println(cfg.ToStr2(data))
	// log.Println("===========================")
	// smap := cfg.ToMap(data, "json", true)
	// log.Println(cfg.ToStr2(smap))
}

// go test -v z/cfg/zcfg_test.go -run Test_tags

func Test_tags(t *testing.T) {
	data := &Data{}
	text, _ := os.ReadFile("zcfg_test.toml")
	cfg.NewTOML(text).Decode(data, "json")
	// tags, _, _ := cfg.ToTagMap(data, "json", true, nil)
	tags := cfg.ToTagVal(data, "json")
	log.Println("===========================")
	log.Println(cfg.ToStr2(tags))
	log.Println("===========================")
	tags[len(tags)-1].Value.Set(reflect.ValueOf(12))
	tags[0].Value.Set(reflect.ValueOf(true))
	log.Println(cfg.ToStr2(data))
}

// go test -v z/cfg/zcfg_test.go -run Test_StrToArr

func Test_StrToArr(t *testing.T) {
	str := `["a","b"]`
	arr := cfg.ToStrArr(str)
	log.Println(arr)

	str = `[1, 2]`
	arr = cfg.ToStrArr(str)
	log.Println(arr)

	str = `[a, b]`
	arr = cfg.ToStrArr(str)
	log.Println(arr)

	str = `['a', 'b']`
	arr = cfg.ToStrArr(str)
	log.Println(arr)

	str = `123.123`
	aaa := cfg.ToStrOrArr(str)
	log.Println(aaa)

	str = `"123.123"`
	aaa = cfg.ToStrOrArr(str)
	log.Println(aaa)
	log.Println("======================")
	str = `["x", 1, true, 'y', "a'b", 'c"d', "e\\f"]`
	arr = cfg.ToStrArr(str)
	for _, v := range arr {
		log.Println(v)
	}
	log.Println("======================")
	str = `["x", 1, true, 'y', "a'b", 'c"d', "e\\f"]`
	aaa = cfg.ToStrOrArr(str)
	log.Println(aaa)

	log.Println("======================")
	str = `["x", "a'b", "e\\f"]`
	aaa = cfg.ToStrOrArr(str)
	log.Println(aaa)

	log.Println("======================")

}

func Test_arr(t *testing.T) {
	arr := []string{"a", "b", "c"}
	log.Println(arr)
	// log.Println(arr[:-1])
}

// go test -v z/cfg/zcfg_test.go -run Test_ToBasicVal

func Test_ToBasicVal(t *testing.T) {
	str := []string{"123", "456"}
	var val any
	var err error
	val, err = cfg.ToBasicValue(reflect.TypeOf(float32(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(float64(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(int(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(int8(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(int32(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(int64(0)), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(""), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf([]byte("")), str)
	log.Printf("=======%#v | %v\n", string(val.([]byte)), err)
	val, err = cfg.ToBasicValue(reflect.TypeOf(str), str)
	log.Printf("=======%#v | %v\n", val, err)

	val, err = cfg.ToBasicValue(reflect.TypeOf([]int{}), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf([]int32{}), str)
	log.Printf("=======%#v | %v\n", val, err)
	val, err = cfg.ToBasicValue(reflect.TypeOf([][]byte{}), str)
	log.Printf("=======%#v | %v\n", string(val.([][]byte)[0]), err)

}

type Record struct {
	NameKey string
	AgeKey  int

	DataKey RecordData
	DataKe2 *RecordData
}

func (r Record) MarshalJSON() ([]byte, error) {
	return cfg.ToJsonBytes(&r, "json", cfg.LowerFirst, false)
}

type RecordData struct {
	NameKey string
	AgeKey  int
}

func (r RecordData) MarshalJSON() ([]byte, error) {
	return cfg.ToJsonBytes(&r, "json", cfg.Camel2Case, true)
}

// go test -v z/cfg/zcfg_test.go -run Test_ToJson

func Test_ToJson(t *testing.T) {
	record := Record{
		NameKey: "x",
		AgeKey:  12,
		DataKey: RecordData{
			NameKey: "",
			AgeKey:  13,
		},
	}
	log.Println(cfg.ToStr2(record))
}
