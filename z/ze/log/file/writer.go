package logfile

import (
	"fmt"
	"sync"

	"github.com/suisrc/zgg/z"
)

type Writer struct {
	AbsPath string // 根路径, 默认 ./logs
	MaxSize int64  // 文件大小限制， 默认 10MB

	files sync.Map
}

func (aa *Writer) Write(fkey string, bts ...[]byte) (int, error) {
	file, exist := aa.files.Load(fkey)
	if !exist {
		file, _ = aa.files.LoadOrStore(fkey, &RollingFile{
			CloseFunc: aa.delfile,
			AbsPath:   aa.AbsPath,
			MaxSize:   aa.MaxSize,
			FileKey:   fkey,
		})
	}
	if rf, ok := file.(*RollingFile); ok {
		return rf.Writex(bts...)
	}
	return 0, fmt.Errorf("invalid file handle type")
}

func (aa *Writer) Close() error {
	aa.files.Range(func(key, value any) bool {
		if lf, ok := value.(*RollingFile); ok {
			lf.Close()
		}
		return true
	})
	aa.files.Clear()
	return nil
}

func (aa *Writer) delfile(lf *RollingFile) {
	aa.files.Delete(lf.FileKey)
	z.Logf("[_logfile]: recycle handle -> %s%d.txt", lf.FileKey, lf.Index)
}
