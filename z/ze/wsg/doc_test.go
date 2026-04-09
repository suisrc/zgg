package websocket

// https://github.com/gorilla/websocket

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
)

// go test -v z/ze/websocket/doc_test.go -run TestSync
func TestSync(t *testing.T) {
	//
	base_url := "https://github.com/gorilla/websocket/tree/main/"
	sync_map := map[string]string{
		"client.go":      "",
		"compression.go": "",
		"conn.go":        "",
		"join.go":        "",
		"json.go":        "",
		"mask.go":        "",
		"prepared.go":    "",
		"proxy.go":       "",
		"server.go":      "",
		"util.go":        "",
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
		src = bytes.ReplaceAll(src, []byte(`interface{}`), []byte(`any`))
		src = bytes.ReplaceAll(src, []byte(`	"io/ioutil"`), []byte(`	"io"`))
		src = bytes.ReplaceAll(src, []byte(`ioutil.`), []byte(`io.`))
		src = bytes.ReplaceAll(src, []byte(`io.ReadFile`), []byte(`os.ReadFile`))
		src = bytes.ReplaceAll(src, []byte(`reflect.PtrTo(`), []byte(`reflect.PointerTo(`))
		err = os.WriteFile(target, src, 0644)
		if err != nil {
			t.Fatal(err)
		}
		println("[__sync__]:", target, "---->", source, "[success]")
	}
}
