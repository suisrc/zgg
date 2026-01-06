package kwlog2

// 使用 fluentbit 收集日志，  详见 fluentbit.yml
// kwlog2 是为了最小化对 fluentbit 日志存储和显示
// 1. 接受日志，写入文件
// 2. 通过列表，展示日志

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var (
	C = struct {
		Kwlog2 Kwlog2Config
	}{}
)

type Kwlog2Config struct {
	Token     string `json:"token"` // 上次日志令牌
	StorePath string `json:"store"` // 文件系统文件夹， 比如 /www, 必须是 / 开头
	RoutePath string `json:"route"` // 访问跟路径
	MaxSize   int64  `json:"max_size"`
	UseOrigin bool   `json:"use_origin"`
}

// 初始化方法， 处理 api 的而外配置接口
type InitializFunc func(api *Kwlog2Api, zgg *z.Zgg)

func Init3(ifn InitializFunc) {
	zc.Register(&C)

	flag.StringVar(&C.Kwlog2.StorePath, "logstore", "logs", "日志存储路径")
	flag.StringVar(&C.Kwlog2.RoutePath, "logroute", "api/logs", "路由访问路径")
	flag.StringVar(&C.Kwlog2.Token, "logtoken", "", "存储日志秘钥")
	flag.BoolVar(&C.Kwlog2.UseOrigin, "logorigin", false, "保存原始数据")
	flag.Int64Var(&C.Kwlog2.MaxSize, "logmaxsize", 10*1024*1024, "日志文件最大大小, 默认 10M")

	z.Register("31-kwlog2", func(zgg *z.Zgg) z.Closed {
		if !z.IsDebug() && strings.Contains(C.Kwlog2.StorePath, "../") {
			zgg.ServeStop("logstore path error, contains '../':", C.Kwlog2.StorePath)
			return nil
		}
		if C.Kwlog2.RoutePath == "" {
			zgg.ServeStop("logroute path error, is empty")
			return nil
		}

		rp := C.Kwlog2.RoutePath
		if rp[0] == '/' {
			rp = rp[1:]
		}
		api := &Kwlog2Api{Config: C.Kwlog2}
		api.Config.RoutePath = "/" + rp
		api.Config.StorePath, _ = filepath.Abs(C.Kwlog2.StorePath)
		zc.Printf("[logstore]: store-path -> %s", api.Config.StorePath)
		api.HttpFS = http.FS(os.DirFS(api.Config.StorePath))
		// zc.Println(zc.ToStr2(C.Kwlog2), zc.ToStr2(api.Config))

		zgg.AddRouter("GET "+rp, api.lst)
		if api.Config.Token != "" { // 增加访问令牌
			zgg.AddRouter("POST "+rp, z.TokenAuth(&api.Config.Token, api.add))
		}
		// zgg.AddRouter("GET favicon.ico", z.Favicon)

		if ifn != nil {
			ifn(api, zgg) // 初始化方法
		}
		return nil
	})
}

type Kwlog2Api struct {
	Config Kwlog2Config

	HttpFS http.FileSystem // 文件系统, http.FS(wwwFS)
	_files sync.Map
}
