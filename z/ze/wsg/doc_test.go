package wsg_test

// https://github.com/gorilla/websocket

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
)

// go test -v z/ze/wsg/doc_test.go -run TestSync
func TestSync(t *testing.T) {
	//
	base_url := "https://raw.githubusercontent.com/gorilla/websocket/refs/heads/main/"
	sync_map := map[string]string{
		"client.go":      "",
		"compression.go": "",
		"conn.go":        "",
		"join.go":        "",
		"json.go":        "",
		"mask.go":        "",
		"prepared.go":    "",
		"server.go":      "",
		"util.go":        "",
		"proxy.go":       "", // 为了拖累三方依赖， 这里的 外部 proxy 被删除
	}

	for target, source := range sync_map {
		if source == "" {
			source = target
		}
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
		src = bytes.ReplaceAll(src, []byte(`package websocket`), []byte(`package wsg`))
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
