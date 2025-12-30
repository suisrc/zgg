package fluent

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
		Fluent FluentConfig
	}{}
)

type FluentConfig struct {
	StorePath string `json:"store"`
	RoutePath string `json:"route"`
	Token     string `json:"token"`
	UseOrigin bool   `json:"use_origin"`
	MaxSize   int64  `json:"max_size"`
}

func Init() {
	zc.Register(&C)

	flag.StringVar(&C.Fluent.StorePath, "logstore", "logs", "日志存储路径")
	flag.StringVar(&C.Fluent.RoutePath, "logroute", "api/logs", "路由访问路径")
	flag.StringVar(&C.Fluent.Token, "logtoken", "", "存储日志秘钥")
	flag.BoolVar(&C.Fluent.UseOrigin, "logorigin", false, "保存原始数据")
	flag.Int64Var(&C.Fluent.MaxSize, "logmaxsize", 10*1024*1024, "日志文件最大大小, 默认 10M")

	z.Register("03-fluent", func(srv z.IServer) z.Closed {
		if !zc.C.Debug && strings.Contains(C.Fluent.StorePath, "../") {
			z.Printf("logstore path error, contains '../': %s", C.Fluent.StorePath)
			srv.ServeStop()
			return nil
		}
		if C.Fluent.RoutePath == "" {
			z.Printf("logroute path error, is empty")
			srv.ServeStop()
			return nil
		}

		rp := C.Fluent.RoutePath
		if rp[0] == '/' {
			rp = rp[1:]
		}
		api := &FluentApi{
			Token:     C.Fluent.Token,
			StorePath: C.Fluent.StorePath,
			RoutePath: "/" + rp,
		}
		api.AbsPath, _ = filepath.Abs(C.Fluent.StorePath)
		z.Printf("[logstore]: store-path -> %s", api.AbsPath)
		api.HttpFS = http.FS(os.DirFS(api.AbsPath))

		srv.AddRouter("GET "+rp, api.lst)
		if C.Fluent.Token != "" { // 增加访问令牌
			srv.AddRouter("POST "+rp, z.TokenAuth(&api.Token, api.add))
		}
		return nil
	})
}

type FluentApi struct {
	Token     string          // 上次日志令牌
	StorePath string          // 文件系统文件夹， 比如 /www, 必须是 / 开头
	RoutePath string          // 访问跟路径
	HttpFS    http.FileSystem // 文件系统, http.FS(wwwFS)
	AbsPath   string
	_files    sync.Map
}
