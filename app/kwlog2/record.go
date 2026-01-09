package kwlog2

import (
	"encoding/json"
	"strings"

	"github.com/suisrc/zgg/z/zc"
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
	if rc.Message != nil && !C.Kwlog2.UseOrigin {
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
			rc.Origin, _ = zc.UnicodeToRunes([]byte(pre), []byte(str))
		} else if bts, err := json.Marshal(rc.Message); err == nil {
			rc.Origin = bts // 重新被json化的数据
		}
	}
	if rc.Origin == nil {
		// 存在风险，需要转移
		rc.Origin, err = zc.UnicodeToRunes(data)
	}
	return err
}
