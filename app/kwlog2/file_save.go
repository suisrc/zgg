package kwlog2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
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
		z.Logf("[logstore]: unmarshal body error, %s", err.Error())
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
		// 转换时区为当前系统时区
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
			date.Year(), date.Month(), date.Format(time.DateOnly)) // time.RFC3339 ? 缺少微秒， 日志统计到微妙
		// 将日志写入Writer中，日志格式为： [时间]-[容器名称]: 日志内容
		fpre := fmt.Sprintf("[%s]-[%s]: ", date.Format(C.Kwlog2.LogTime), rc.PodName)
		aa.Writer.Write(fkey, []byte(fpre), rc.Origin, []byte("\n"))
	}
}
