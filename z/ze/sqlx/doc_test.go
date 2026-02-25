package sqlx_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/suisrc/zgg/z"
)

// go test -v z/ze/sqlx/doc_test.go -run TestSyncSqlx
func TestSyncSqlx(t *testing.T) {
	base_url := "https://raw.githubusercontent.com/jmoiron/sqlx/refs/heads/master/"
	sync_map := map[string]string{
		"types.go":     "types/types.go",
		"reflect.go":   "reflectx/reflect.go",
		"sqlx_bind.go": "bind.go",
	}

	for target, source := range sync_map {
		z.Println("[__sync__]:", target, "---->", source, "[sync...]")
		resp, err := http.Get(base_url + source)
		if err != nil {
			t.Fatal(err)
		}
		src, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		src = bytes.ReplaceAll(src, []byte(`package types`), []byte(`package sqlx`))
		src = bytes.ReplaceAll(src, []byte(`	"io/ioutil"`), []byte(`	"io"`))
		src = bytes.ReplaceAll(src, []byte(`ioutil.`), []byte(`io.`))
		src = bytes.ReplaceAll(src, []byte(`interface{}`), []byte(`any`))
		src = bytes.ReplaceAll(src, []byte(`	"github.com/jmoiron/sqlx/reflectx"`), []byte(``))
		src = bytes.ReplaceAll(src, []byte(`reflectx.`), []byte(``))
		err = os.WriteFile(target, src, 0644)
		if err != nil {
			t.Fatal(err)
		}
		z.Println("[__sync__]:", target, "---->", source, "[success]")
	}

}
