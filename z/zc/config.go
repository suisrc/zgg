// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

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
	cs = map[string]any{}
)

// Config 配置参数
type Config struct {
	Debug bool `default:"false" json:"debug"`
	Print bool `default:"false" json:"printconfig"`
}

var (
	CFG_TAG = "json" // 自定义标签配置名称
	CFG_ENV = "zgg"  // 自定义环境变量前缀
	load    sync.Once
)

type ILoader interface {
	Load(any) error
}

// --------------------------------------------------------------------------------

func Register(c any) {
	ctype := reflect.TypeOf(c)
	if ctype.Kind() != reflect.Pointer {
		panic("z/zc: Register c(arg) must be pointer")
	}
	cs[fmt.Sprintf("%v.%p", ctype.Elem(), c)] = c
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
		for _, conf := range cs {
			for _, loader := range loaders {
				if err := loader.Load(conf); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(2)
				}
			}
		}
	})
	if C.Print {
		for name, conf := range cs {
			println("--------" + name)
			println(ToStr2(conf))
		}
		println("----------------------------------------------")
	}
}
