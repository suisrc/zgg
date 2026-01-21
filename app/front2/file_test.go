package front2_test

import (
	"os"
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
