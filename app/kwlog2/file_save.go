package kwlog2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suisrc/zgg/z"
	"github.com/suisrc/zgg/z/zc"
)

var (
	// 日志标签
	HeaderTagKey = "X-Request-Tag"
	ParamBodyErr = &z.Result{Success: false, ErrShow: 1, ErrCode: "PARAM-BODY-ERR", Message: "请求参数错误, 无法解析", Status: http.StatusBadRequest}
	SuccessOK    = &z.Result{Success: true, Data: "ok"}
)

func (aa *Kwlog2Api) add(zrc *z.Ctx) {
	logs := []Record{}
	if err := json.NewDecoder(zrc.Request.Body).Decode(&logs); err != nil {
		zc.Printf("[logstore]: unmarshal body error, %s", err.Error())
		zrc.JSON(ParamBodyErr)
		return
	}
	ktag := zrc.Request.Header.Get(HeaderTagKey)
	go aa.log(logs, ktag) // 异步写入
	zrc.Writer.WriteHeader(http.StatusOK)
}

func (aa *Kwlog2Api) log(rcs []Record, ktag string) {
	for _, rc := range rcs {
		if rc.AppName == "" {
			rc.AppName = rc.PodName
		}
		if rc.Namespace == "" {
			rc.Namespace = "unknown"
		}
		if rc.AppName == "" {
			rc.AppName = "unknown"
		}
		if rc.PodName == "" {
			rc.PodName = "unknown"
		}
		if rc.Time == 0 {
			rc.Time = float64(time.Now().UnixMicro()) / 1_000_000
		}
		date := time.UnixMicro(int64(rc.Time * 1_000_000))
		// ----------------------------------------------------
		if ktag != "" {
			if tags := strings.SplitN(ktag, ".", 3); len(tags) >= 2 {
				if len(tags) == 1 {
					ktag = tags[0]
				} else {
					ktag = fmt.Sprintf("%s.%s", tags[0], tags[1])
				}
			}
			ktag = "/" + ktag
		}
		// /ktag/namespace/appname/%Y/%M/%Y-%M-%D_0.txt
		fkey := fmt.Sprintf("%s/%s/%s/%02d/%02d/%s_", ktag, rc.Namespace, rc.AppName, //
			date.Year(), date.Month(), date.Format(time.DateOnly))
		// file := fpre + "0.txt"
		file, ok := aa._files.Load(fkey)
		if !ok {
			file, _ = aa._files.LoadOrStore(fkey, &LoggerFile{
				DelFunc: aa.del_file,
				AbsPath: aa.AbsPath,
				FileKey: fkey,
				MaxSize: aa.MaxSize,
			})
		}
		tpre := fmt.Sprintf("[%s]-[%s]: ", date.Format(time.RFC3339), rc.PodName)
		file.(*LoggerFile).Write([]byte(tpre), []byte(rc.Origin), []byte{'\n'})
	}
}

func (aa *Kwlog2Api) del_file(lf *LoggerFile) {
	aa._files.Delete(lf.FileKey)
	zc.Printf("[logstore]: recycle handle -> %s%d.txt", lf.FileKey, lf.Index)
}

type LoggerFile struct {
	DelFunc func(*LoggerFile)

	MaxSize int64  // 文件大小限制， 默认 10MB
	AbsPath string // 根路径
	FileKey string // 文件键
	FileHdl *os.File

	Index int         // 文件索引
	mlock sync.Mutex  // 写入锁定
	timer *time.Timer // 是否存在
	alive int64       // 存活时间
}

func (aa *LoggerFile) Write(bts ...[]byte) {
	aa.mlock.Lock()
	defer aa.mlock.Unlock()
	// defer aa.close()
	if aa.FileHdl != nil {
		if fstat, _ := aa.FileHdl.Stat(); fstat != nil && fstat.Size() > aa.MaxSize {
			// 文件大小超过限制， 关闭文件句柄
			aa.FileHdl.Close()
			aa.FileHdl = nil
			aa.Index++
		} else {
			// 复用文件句柄，写入文件
			defer aa.close()
			for _, bt := range bts {
				aa.FileHdl.Write(bt)
			}
			return
		}
	}
	fpath := ""
	fpkey := filepath.Join(aa.AbsPath, aa.FileKey)
	for {
		fpath = fmt.Sprintf("%s%d.txt", fpkey, aa.Index)
		if fstat, err := os.Stat(fpath); err != nil && os.IsNotExist(err) {
			// 文件不存在， 创建文件所在的文件夹
			parent := filepath.Dir(fpath)
			if _, err := os.Stat(parent); os.IsNotExist(err) {
				os.MkdirAll(parent, 0644)
			}
			break
		} else if err == nil && fstat.Size() > aa.MaxSize {
			aa.Index++
			continue
		} else if err == nil && fstat.IsDir() {
			zc.Printf("[logstore]: check store file error -> %s, %s", fpath, " is dir")
			aa.DelFunc(aa)
			return // 跳过，文件名存在同名文件夹
		} else if err != nil {
			zc.Printf("[logstore]: check store file error -> %s, %s", fpath, err.Error())
			aa.DelFunc(aa)
			return // 跳过，无法处理，遇到不可预知错误
		} else {
			// 文件存在，而且大小合适， 继续写入
			break
		}
	}
	var err error // 创建 + 追加 + 只写
	aa.FileHdl, err = os.OpenFile(fpath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		zc.Printf("[logstore]: open store file error -> %s, %s", fpath, err.Error())
		aa.DelFunc(aa)
		return // 跳过，无法处理， 无法打开或者创建文件夹
	}
	defer aa.close()
	for _, bt := range bts {
		aa.FileHdl.Write(bt)
	}

}

func (aa *LoggerFile) close() {
	aa.alive = time.Now().Unix() + 10
	if aa.timer != nil {
		return // 执行器存在， 跳过
	}
	// 创建回收器， 延迟关闭
	aa.timer = time.AfterFunc(time.Second*5, aa._close)
}
func (aa *LoggerFile) _close() {
	if aa.alive > time.Now().Unix() {
		// 创建回收器，继续迭代检查
		aa.timer.Reset(time.Second * 5)
		return
	}
	aa.DelFunc(aa)
	aa.FileHdl.Close()
	aa.FileHdl = nil
	aa.timer = nil // 删除执行器
}
