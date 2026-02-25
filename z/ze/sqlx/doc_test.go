package sqlx_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
)

// go test -v z/ze/sqlx/doc_test.go -run TestSyncSqlx
func TestSyncSqlx(t *testing.T) {
	base_url := "https://raw.githubusercontent.com/jmoiron/sqlx/refs/heads/master/"
	sync_map := map[string]string{
		"types.go":          "types/types.go",
		"reflect.go":        "reflectx/reflect.go",
		"sqlx_bind.go":      "bind.go",
		"sqlx.go":           "sqlx.go",
		"sqlx_named.go":     "named.go",
		"sqlx_context.go":   "sqlx_context.go",
		"sqlx_named_ctx.go": "named_context.go",
	}

	for target, source := range sync_map {
		println("[__sync__]:", target, "---->", source, "[sync...]")
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
		src = bytes.ReplaceAll(src, []byte(`package reflectx`), []byte(`package sqlx`))
		src = bytes.ReplaceAll(src, []byte(`	"io/ioutil"`), []byte(`	"io"`))
		src = bytes.ReplaceAll(src, []byte(`ioutil.`), []byte(`io.`))
		src = bytes.ReplaceAll(src, []byte(`io.ReadFile`), []byte(`os.ReadFile`))
		src = bytes.ReplaceAll(src, []byte(`interface{}`), []byte(`any`))
		src = bytes.ReplaceAll(src, []byte(`	"github.com/jmoiron/sqlx/reflectx"`), []byte(``))
		src = bytes.ReplaceAll(src, []byte(`reflectx.`), []byte(``))
		src = bytes.ReplaceAll(src, []byte(`reflect.PtrTo(`), []byte(`reflect.PointerTo(`))
		src = bytes.ReplaceAll(src, []byte(`mustBe`), []byte(`MustBe`))
		err = os.WriteFile(target, src, 0644)
		if err != nil {
			t.Fatal(err)
		}
		println("[__sync__]:", target, "---->", source, "[success]")
	}

}
