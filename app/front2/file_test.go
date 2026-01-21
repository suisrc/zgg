package front2_test

import (
	"os"
	"strings"
	"testing"

	"github.com/suisrc/zgg/app/front2"
	"github.com/suisrc/zgg/z"
)

// go test -v app/front2/file_test.go -run Test_files

func Test_files(t *testing.T) {
	fim, _ := front2.GetFileMap(os.DirFS("../../www"))
	for k, v := range fim {
		z.Printf("%-50s | %-20s | %v", k, v.Name(), v.IsDir())
	}
	z.Println("===============================")
}

// go test -v app/front2/file_test.go -run Test_result

func Test_result(t *testing.T) {
	result := "success"

	res := "{\"success\":false, \"data\":0,\"showType\": 9}"
	if idx := strings.Index(res, "\"success\":"); idx > 0 && idx+11 < len(res) {
		idx += 10
		if res[idx] == 'f' {
			if strings.Contains(res, "\"showType\":9") || strings.Contains(res, "\"errshow\":9") {
				result = "redirect"
			} else {
				result = "abnormal"
			}
		} else if res[idx] == ' ' && res[idx+1] == 'f' {
			if strings.Contains(res, "\"showType\": 9") || strings.Contains(res, "\"errshow\": 9") {
				result = "redirect"
			} else {
				result = "abnormal"
			}
		}
	}
	z.Println("===============================", result)
}
