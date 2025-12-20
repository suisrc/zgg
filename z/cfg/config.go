// Copyright 2025 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package cfg

import (
	"log"
	"os"
	"strings"
	"sync"
)

func init() {
	cs = append(cs, C)
}

var (
	// C 全局配置(需要先执行MustLoad，否则拿不到配置)
	C = new(Config)
	// cs 配置对象集合
	cs = []any{}
)

// Config 配置参数
type Config struct {
	Debug bool `default:"false"`
	Print bool `default:"false" json:"printconfig"`
}

// --------------------------------------------------------------------------------

func Register(c any) {
	cs = append(cs, c)
}

// PrintConfig 打印配置
func PrintConfig() {
	if C.Print {
		os.Stdout.WriteString(ToStr2(cs) + "\n")
	}
}

// --------------------------------------------------------------------------------

var (
	CFG_TAG = "json" // 自定义标签配置名称
	CFG_ENV = "zgg"  // 自定义环境变量前缀
	load    sync.Once
)

type ILoader interface {
	Load(any) error
}

// MustLoad 加载配置, 这是一个简单的配置加载器
// 如果发生无法解决的问题，可以使用 github.com/koding/multiconfig 替换
func MustLoad(fpaths ...string) {
	load.Do(func() {
		loaders := []ILoader{NewTAG()}
		for _, fpath := range fpaths {
			fpath = strings.TrimSpace(fpath)
			if fpath == "" {
				continue
			}
			// load config file
			if data, err := os.ReadFile(fpath); err == nil {
				loaders = append(loaders, NewTOML(data))
			} else {
				log.Println("z/cfg: read file error, ", err.Error())
			}
		}
		loaders = append(loaders, NewENV(CFG_ENV))
		for _, loader := range loaders {
			for _, conf := range cs {
				loader.Load(conf)
			}
		}
	})
}

// 基于 github.com/koding/multiconfig 加载配置
// func MustLoad(fpaths ...string) {
// 	load.Do(func() {
// 		loaders := []multiconfig.Loader{&multiconfig.TagLoader{}}
// 		for _, fpath := range fpaths {
// 			//if strings.HasSuffix(fpath, "ini") {
// 			//	loaders = append(loaders, &multiconfig.INILLoader{Path: fpath})
// 			//}
// 			if strings.HasSuffix(fpath, "toml") {
// 				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
// 			}
// 			if strings.HasSuffix(fpath, "json") {
// 				loaders = append(loaders, &multiconfig.JSONLoader{Path: fpath})
// 			}
// 			if strings.HasSuffix(fpath, "yml") || strings.HasSuffix(fpath, "yaml") {
// 				loaders = append(loaders, &multiconfig.YAMLLoader{Path: fpath})
// 			}
// 		}
// 		loaders = append(loaders, &multiconfig.EnvironmentLoader{Prefix: strings.ToUpper(CFG_ENV)})

// 		m := multiconfig.DefaultLoader{
// 			Loader:    multiconfig.MultiLoader(loaders...),
// 			Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
// 		}
// 		// 加载配置
// 		for _, conf := range cs {
// 			m.MustLoad(conf)
// 		}
// 	})
// }
