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
	GetFileMap = _GetFileMap // 获取文件列表
	GetFixFile = _GetFixFile // 获取文件内容
	IsFixFile  = _IsFixFile  // 判断是否需要修复文件
	FixReqPath = _FixReqPath // 修正请求路径
)

func _GetFileMap(fsys fs.FS) (map[string]fs.FileInfo, error) {
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

func _GetFixFile(hf http.File, fp, tp, rp string, fm map[string]fs.FileInfo) ([]byte, error) {
	tbts, err := io.ReadAll(hf) // 需要转换内容 tp -> rp
	if err != nil {
		return nil, err
	} else if tp == "" {
		return tbts, nil
	} else if tp[0] == '@' {
		tp = tp[1:] // 忽略开头的 @, 跳过， 使用基础处理
	} else if tp != "/" {
		return bytes.ReplaceAll(tbts, []byte(tp), []byte(rp)), nil
	}
	if tp[len(tp)-1] == '/' {
		tp = tp[:len(tp)-1]
	}
	if len(rp) > 0 && rp[len(rp)-1] == '/' {
		rp = rp[:len(rp)-1]
	}
	// 备用替换方式
	if fm != nil && (strings.HasSuffix(fp, ".html") || strings.HasSuffix(fp, ".htm")) {
		for k := range fm {
			// 执行内容替换
			tp_ := []byte("\"" + tp + "/" + k + "\"")
			rp_ := []byte("\"" + rp + "/" + k + "\"")
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
	tp_ := []byte(".p=\"" + tp + "/\"")
	rp_ := []byte(".p=\"" + rp + "/\"")
	return bytes.ReplaceAll(tbts, tp_, rp_), nil
}

func _IsFixFile(name string, conf *Config) bool {
	if conf.TmplRoot == "" || conf.TmplRoot == "none" {
		return false
	}
	for _, suf := range conf.TmplSuffix {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	for _, pre := range conf.TmplPrefix {
		if strings.HasPrefix(name, pre) {
			return true
		}
	}
	return false
}

func _FixReqPath(rr *http.Request, roots []string, dir string) string {
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
