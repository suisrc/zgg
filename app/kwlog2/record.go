package kwlog2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/suisrc/zgg/z"
)

type Record0 struct {
	Time      float64 `json:"__time"`
	Namespace string  `json:"__namespace_name"`
	AppName   string  `json:"__app_name"`
	PodName   string  `json:"__pod_name"`
	CriName   string  `json:"__container_name"`
	CriImage  string  `json:"__container_image"`
	Message   any     `json:"message"`
}

type Record struct {
	Record0
	Origin []byte `json:"-"` // 原始数据
}

func (rc *Record) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &rc.Record0)
	if err != nil {
		return err
	}
	// if str, ok := rc.Message.(string); ok && len(str) > 0 && str[0] == '{' {
	// 	map_ := map[string]any{}
	// 	if err := json.Unmarshal([]byte(str), &map_); err == nil {
	// 		rc.Message = map_ // 尝试解析 message 内容
	// 	}
	// }
	if rc.Message != nil && !G.Kwlog2.UseOrigin {
		if str, ok := rc.Message.(string); ok {
			// 消息将被替换，补充一些容器信息
			pre := ""
			if rc.CriImage != "" {
				idx := strings.LastIndexByte(rc.CriImage, '/')
				if idx > 0 {
					pre = "(" + rc.CriImage[idx+1:] + ") "
				}
			}
			// rc.Origin = []byte(str) // 防止 unicode 字符存在
			rc.Origin, _ = z.UnicodeTo([]byte(pre), []byte(str))
		} else if bts, err := json.Marshal(rc.Message); err == nil {
			rc.Origin = bts // 重新被json化的数据
		}
	}
	if rc.Origin == nil {
		// 存在风险，需要转移
		rc.Origin, err = z.UnicodeTo(data)
	}
	return err
}

//----------------------------------------------------------------------------------------------------------------

var (
	// 日志标签
	HeaderTagKey = "X-Request-Tag"
	ParamBodyErr = &z.Result{Success: false, ErrShow: 1, ErrCode: "PARAM-BODY-ERR", Message: "请求参数错误, 无法解析", Status: http.StatusBadRequest}
	SuccessOK    = &z.Result{Success: true, Data: "ok"}
)

// 记录日志
func (aa *KwlogHandler) AddRecord(zrc *z.Ctx) {
	logs := []Record{}
	if err := json.NewDecoder(zrc.Request.Body).Decode(&logs); err != nil {
		z.Logf("[logstore]: unmarshal body error, %s", err.Error())
		zrc.JSON(ParamBodyErr)
		return
	}
	ktag := zrc.Request.Header.Get(HeaderTagKey)
	go aa.record(logs, ktag) // 异步写入
	zrc.Writer.WriteHeader(http.StatusOK)
}

// 记录日志
func (aa *KwlogHandler) record(rcs []Record, ktag string) {
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
		fpre := fmt.Sprintf("[%s]-[%s]: ", date.Format(G.Kwlog2.LogTime), rc.PodName)
		aa.Writer.Write(fkey, []byte(fpre), rc.Origin, []byte("\n"))
	}
}
