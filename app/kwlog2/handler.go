package kwlog2

// 使用 fluentbit 收集日志，  详见 fluentbit.yml
// kwlog2 是为了最小化对 fluentbit 日志存储和显示
// 1. 接受日志，写入文件
// 2. 通过列表，展示日志

import (
	"flag"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
	logfile "github.com/suisrc/zgg/z/ze/log/file"
)

var (
	G = struct {
		Kwlog2 Config
	}{}
)

type Config struct {
	Token     string `json:"token"` // 上次日志令牌
	StorePath string `json:"store"` // 文件系统文件夹， 比如 /www, 必须是 / 开头
	RoutePath string `json:"route"` // 访问跟路径
	MaxSize   int64  `json:"max_size"`
	UseOrigin bool   `json:"use_origin"`
	LogTime   string `json:"log_time" default:"2006-01-02T15:04:05.000Z07:00"`
}

// 初始化方法， 处理 hdl 的而外配置接口
type InitFunc func(hdl *KwlogHandler, zgg *z.Zgg)

func Init3(ifn InitFunc) {
	z.Config(&G)

	flag.StringVar(&G.Kwlog2.StorePath, "logstore", "logs", "日志存储路径")
	flag.StringVar(&G.Kwlog2.RoutePath, "logroute", "api/logs", "路由访问路径")
	flag.StringVar(&G.Kwlog2.Token, "logtoken", "", "存储日志秘钥")
	flag.BoolVar(&G.Kwlog2.UseOrigin, "logorigin", false, "保存原始数据")
	flag.Int64Var(&G.Kwlog2.MaxSize, "logmaxsize", 10*1024*1024, "日志文件最大大小, 默认 10M")
	flag.StringVar(&G.Kwlog2.LogTime, "logtimerfc", time.RFC3339, "日志时间格式")

	z.Register("31-kwlog2", func(zgg *z.Zgg) z.Closed {
		if !z.IsDebug() && strings.Contains(G.Kwlog2.StorePath, "../") {
			zgg.ServeStop("logstore path error, contains '../':", G.Kwlog2.StorePath)
			return nil
		}
		if G.Kwlog2.RoutePath == "" {
			zgg.ServeStop("logroute path error, is empty")
			return nil
		}
		rpath := G.Kwlog2.RoutePath
		if rpath[0] == '/' {
			rpath = rpath[1:]
		}
		hdl := &KwlogHandler{Config: G.Kwlog2}
		hdl.Config.RoutePath = "/" + rpath
		hdl.Config.StorePath, _ = filepath.Abs(G.Kwlog2.StorePath)
		z.Logf("[logstore]: store-path -> %s", hdl.Config.StorePath)
		hdl.HttpFS = http.FS(os.DirFS(hdl.Config.StorePath))
		// z.Logn(zc.ToStrJSON(G.Kwlog2), zc.ToStrJSON(hdl.Config))
		mime.AddExtensionType(".log", "text/plain")
		zgg.AddRouter("GET "+rpath, hdl.ShowFiles) // 显示列表日志
		if hdl.Config.Token != "" {                // 增加访问令牌
			zgg.AddRouter("POST "+rpath, z.TokenAuth(&hdl.Config.Token, hdl.AddRecord))
		}
		// zgg.AddRouter("GET favicon.ico", z.Favicon)
		hdl.Writer = &logfile.Writer{AbsPath: hdl.Config.StorePath, MaxSize: hdl.Config.MaxSize}
		if ifn != nil {
			ifn(hdl, zgg) // 初始化方法
		}
		return nil
	})
}

type KwlogHandler struct {
	Config Config
	HttpFS http.FileSystem // 文件系统, http.FS(wwwFS)
	Writer *logfile.Writer // 日志写入器
}
