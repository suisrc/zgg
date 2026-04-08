package front2

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// 便于替换默认方法
var (
	GetRefFileMap = _GetRefFileMap // 获取文件列表
	GetFixFileRef = _GetFixFileRef // 获取文件内容
	CanFixFileRef = _CanFixFileRef // 校验文件内容
	FixReqUrlPath = _FixReqUrlPath // 修正请求路径
)

// 获取文件列表
func _GetRefFileMap(fsys fs.FS) (map[string]fs.FileInfo, error) {
	fim := make(map[string]fs.FileInfo)
	err := fs.WalkDir(fsys, ".", func(path string, file fs.DirEntry, err error) error {
		if err != nil || file.IsDir() {
			return err
		}
		info, err := file.Info()
		if err != nil {
			return err
		}
		fim[path] = info
		return nil
	})
	return fim, err
}

// fname: 原始文件名称
// tpath: "",   直接跳过, @... 只用基础匹配, "/"  只用基础匹配, /... 直接替换匹配;
// rpath: 替换的目标内容， 不以 "/" 结尾
// tenf: true: 如果 tpath 中纯在 xxx:zzz 格式， 使用 zzz 替代 rpath，否则忽略 zzz
func _GetFixFileRef(hfile http.File, fname, tpath, rpath string, fmap map[string]fs.FileInfo, tenf bool) ([]byte, error) {
	if idx := strings.IndexByte(tpath, ':'); idx > 0 {
		if tenf {
			rpath = tpath[idx+1:]
		}
		tpath = tpath[:idx] // 删除 ：后面的内容
	}
	if len(rpath) > 0 && rpath[len(rpath)-1] == '/' {
		rpath = rpath[:len(rpath)-1] // 删除末尾的 /，如果只是  "/" 就变为 ""
	}
	if len(tpath) > 0 && tpath[len(tpath)-1] == '/' {
		tpath = tpath[:len(tpath)-1] // 基础匹配，就是可能是空白符的情况
	}
	tbts, err := io.ReadAll(hfile) // 需要转换内容 tp -> rp
	if err != nil {
		return nil, err
	} else if tpath == "" || tpath == rpath {
		return tbts, nil
	} else if tpath[0] == '@' {
		tpath = tpath[1:] // 忽略开头的 @, 跳过， 使用基础处理
	} else if tpath != "/" {
		return bytes.ReplaceAll(tbts, []byte(tpath), []byte(rpath)), nil
	}
	if tpath == rpath {
		return tbts, nil // 删除 '@' 后， 参数的错误传递，可能会发生一致
	}
	// 备用替换方式
	if fmap != nil && (strings.HasSuffix(fname, ".html") || strings.HasSuffix(fname, ".htm")) {
		for k := range fmap {
			// 执行内容替换
			tp_ := []byte("\"" + tpath + "/" + k + "\"")
			rp_ := []byte("\"" + rpath + "/" + k + "\"")
			if bytes.Contains(tbts, tp_) {
				// 确认文件需要执行替换操作
				tbts = bytes.ReplaceAll(tbts, tp_, rp_)
			}
			tp_[len(tp_)-1] = '?'
			rp_[len(rp_)-1] = '?'
			if bytes.Contains(tbts, tp_) {
				// 链接可能存在带有参数情况，一并处理
				tbts = bytes.ReplaceAll(tbts, tp_, rp_)
			}
		}
		return tbts, nil
	}
	// 其他备用方式
	tp_ := []byte(".p=\"" + tpath + "/\"")
	rp_ := []byte(".p=\"" + rpath + "/\"")
	return bytes.ReplaceAll(tbts, tp_, rp_), nil
}

// 判断是否需要修正文件内容
func _CanFixFileRef(name string, conf *Config) bool {
	if conf.TmplRoot == "" || conf.TmplRoot == "none" {
		return false
	}
	for _, key := range conf.TmplFile {
		if len(key) == 0 {
			continue
		} else if key[0] == '^' {
			// 前缀匹配
			if strings.HasPrefix(name, key[1:]) {
				return true
			}
		} else if strings.HasSuffix(name, key) {
			// 后缀匹配
			return true
		}
	}
	return false
}

// 修正请求路径， 支持多级路径的修正， 适合 CDN 加载的索引文件
func _FixReqUrlPath(rr *http.Request, roots []string, dir string) string {
	rp := ""
	for _, path := range roots {
		if path == "" {
			continue
		}
		if path[len(path)-1] == '/' && strings.HasPrefix(rr.URL.Path, path) {
			rp = path
			rr.URL.Path = rr.URL.Path[len(rp)-1:]
			rr.URL.RawPath = strings.TrimPrefix(rr.URL.RawPath, rp[:len(rp)-1])
			break
		} else if rr.URL.Path == path || strings.HasPrefix(rr.URL.Path, path+"/") {
			rp = path
			rr.URL.Path = rr.URL.Path[len(rp):]
			rr.URL.RawPath = strings.TrimPrefix(rr.URL.RawPath, rp)
			break
		}
	}
	if dir != "" {
		rr.URL.Path = filepath.Join(dir, rr.URL.Path)
		if rr.URL.RawPath != "" {
			rr.URL.RawPath = filepath.Join(dir, rr.URL.RawPath)
		}
	}
	return rp
}
