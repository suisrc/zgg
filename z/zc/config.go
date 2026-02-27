// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// zc 原本是 zgg config 的简写， 只为对 zgg 的应用进行配置的处理
// 但是随着功能的增多， zc 现在已经成为了 zgg core 层，这属实无奈
// 之后极可能控制新功能的引入， 已保障 zc 的简洁

package zc

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
)

func init() {
	Register(C)
}

var (
	// C 全局配置(需要先执行MustLoad，否则拿不到配置)
	C = new(Config)
	// cs 配置对象集合
	CS = map[string]any{}    // 需要初始化配置
	FS = map[string]func(){} // 配置初始化函数

	InitConfigFn = func() {}
)

// Config 配置参数
type Config struct {
	Debug  bool   `default:"false" json:"debug"`
	Print  bool   `json:"print"`  // 用于调试，打印所有的的参数
	Cache  bool   `json:"cache"`  // 是否启用缓存, 如果启用，可以通过 GetByKey 获取已有的配置
	Syslog string `json:"syslog"` // udp://klog.default.svc:514, syslog 输出地址
	LogTty bool   `json:"logtty"` // 启用 syslog 同步打印日志到控制台
	LogTff bool   `json:"logtcf"` // 追踪打印日志的位置
}

var (
	CFG_TAG = "json" // 自定义标签配置名称
	CFG_ENV = "zgg"  // 自定义环境变量前缀
	load    sync.Once
	vcache  = map[string]reflect.Value{} // 变了缓存
)

type ILoader interface {
	Load(any) error
}

// --------------------------------------------------------------------------------

// Register 注册配置对象， Pointer or Func[func()] 函数，如果有异常，使用 panic/os.Exit(2) 终止
func Register(c any) {
	ctype := reflect.TypeOf(c)
	if ctype.Kind() == reflect.Func {
		fn, ok := c.(func())
		if !ok {
			panic("z/zc: Register f(arg) must be [func()]")
		}
		FS[fmt.Sprintf("%p", c)] = fn
		return
	}
	if ctype.Kind() != reflect.Pointer {
		panic("z/zc: Register c(arg) must be pointer")
	}
	CS[fmt.Sprintf("%v.%p", ctype.Elem(), c)] = c
}

func LoadConfig(cfs string) {
	load.Do(func() {
		// var cfs string
		// flag.StringVar(&cfs, "c", "", "config file path")
		// flag.Parse() // command line arguments
		// ---------------------------------------------------------------

		loaders := []ILoader{NewTAG()} // 通过标签初始化配置
		if cfs != "" {
			// 通过文件加载配置
			for fpath := range strings.SplitSeq(cfs, ",") {
				fpath = strings.TrimSpace(fpath)
				if fpath == "" {
					continue
				}
				// load config file
				if data, err := os.ReadFile(fpath); err == nil {
					loaders = append(loaders, NewTOML(data))
				} else {
					log.Println("z/zc: read file error, ", err.Error())
				}
			}
		}
		loaders = append(loaders, NewENV(CFG_ENV)) // 通过环境加载配置

		// // 如果发生无法解决的问题，可以使用 github.com/koding/multiconfig 替换
		// loaders := []multiconfig.Loader{&multiconfig.TagLoader{}}
		// for fpath := range strings.SplitSeq(cfs, ",") {
		// 	//if strings.HasSuffix(fpath, "ini") {
		// 	//	loaders = append(loaders, &multiconfig.INILLoader{Path: fpath})
		// 	//}
		// 	if strings.HasSuffix(fpath, "toml") {
		// 		loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
		// 	}
		// 	if strings.HasSuffix(fpath, "json") {
		// 		loaders = append(loaders, &multiconfig.JSONLoader{Path: fpath})
		// 	}
		// 	if strings.HasSuffix(fpath, "yml") || strings.HasSuffix(fpath, "yaml") {
		// 		loaders = append(loaders, &multiconfig.YAMLLoader{Path: fpath})
		// 	}
		// }
		// loaders = append(loaders, &multiconfig.EnvironmentLoader{Prefix: strings.ToUpper(CFG_ENV)})
		// // m := multiconfig.DefaultLoader{
		// // 	Loader:    multiconfig.MultiLoader(loaders...),
		// // 	Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
		// // }

		// ---------------------------------------------------------------
		// load config
		for _, conf := range CS {
			for _, loader := range loaders {
				if err := loader.Load(conf); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(2)
				}
			}
		}
		for _, fn := range FS {
			fn()
		}
	})
	if C.Print {
		for name, conf := range CS {
			println("--------" + name)
			println(ToStr2(conf))
		}
		println("----------------------------------------------")
	}
	if !C.Cache {
		vcache = nil // 禁用缓存， 缓存是在 Env 中完成初始化的
	}
	InitConfigFn()
}

// 获取配置文件中指定的字段值， 可能存在 key 相同的覆盖情况， PS: 由于使用的是 reflect.Value，因此原始值改变时，缓存也会改变
func GetByKey[T any](key string, def T) T {
	if vcache == nil {
	} else if vc, ok := vcache[key]; !ok {
	} else if vv, ok := vc.Interface().(T); !ok {
	} else {
		return vv
	}
	return def // 缓存未命中， 返回默认值
}

func GetByPre(pre string) map[string]any {
	rst := map[string]any{}
	for k, v := range vcache {
		if pre == "" || strings.HasPrefix(k, pre) {
			rst[k] = v.Interface()
		}
	}
	return rst
}
