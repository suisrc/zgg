package zc_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/suisrc/zgg/z/zc"
)

// go test -v z/zc/toml_test.go -run TestTOMLParseComplexSyntax

func TestTOMLParseComplexSyntax(t *testing.T) {
	text := []byte(`title = "demo" # inline comment
"service.name" = "api"
dotted.key = "value"
arr = [
  "a",
  "b",
  "c",
]
multiline = """
line1
line2
"""
literal = '''
raw
text
'''
inline = { enabled = true, retry.count = 3, meta = { owner = "ops" }, list = [1, 2] }

[server."alpha.beta"]
host = "127.0.0.1"

[[clients]]
name = "web"
ports = [80, 443]

[[clients]]
name = "ops"
ports = [8080]
`)

	data := zc.NewTOML(text).Map()

	if got := data["title"]; got != "demo" {
		t.Fatalf("title = %v", got)
	}
	if got := data["service.name"]; got != "api" {
		t.Fatalf("service.name = %v", got)
	}
	if got := data["multiline"]; got != "line1\nline2\n" {
		t.Fatalf("multiline = %#v", got)
	}
	if got := data["literal"]; got != "raw\ntext\n" {
		t.Fatalf("literal = %#v", got)
	}

	dotted, ok := data["dotted"].(map[string]any)
	if !ok || dotted["key"] != "value" {
		t.Fatalf("dotted.key = %#v", data["dotted"])
	}

	arr, ok := data["arr"].([]string)
	if !ok || !reflect.DeepEqual(arr, []string{"a", "b", "c"}) {
		t.Fatalf("arr = %#v", data["arr"])
	}

	inline, ok := data["inline"].(map[string]any)
	if !ok {
		t.Fatalf("inline = %#v", data["inline"])
	}
	if got := inline["enabled"]; got != "true" {
		t.Fatalf("inline.enabled = %#v", got)
	}
	retry, ok := inline["retry"].(map[string]any)
	if !ok || retry["count"] != "3" {
		t.Fatalf("inline.retry = %#v", inline["retry"])
	}
	meta, ok := inline["meta"].(map[string]any)
	if !ok || meta["owner"] != "ops" {
		t.Fatalf("inline.meta = %#v", inline["meta"])
	}
	list, ok := inline["list"].([]string)
	if !ok || !reflect.DeepEqual(list, []string{"1", "2"}) {
		t.Fatalf("inline.list = %#v", inline["list"])
	}

	server, ok := data["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %#v", data["server"])
	}
	alpha, ok := server["alpha.beta"].(map[string]any)
	if !ok || alpha["host"] != "127.0.0.1" {
		t.Fatalf("server.alpha.beta = %#v", server["alpha.beta"])
	}

	clients, ok := data["clients"].([]map[string]any)
	if !ok || len(clients) != 2 {
		t.Fatalf("clients = %#v", data["clients"])
	}
	if clients[0]["name"] != "web" || clients[1]["name"] != "ops" {
		t.Fatalf("clients names = %#v", clients)
	}
	if got := clients[0]["ports"]; !reflect.DeepEqual(got, []string{"80", "443"}) {
		t.Fatalf("clients[0].ports = %#v", got)
	}

	t.Log(zc.ToStrJSON(data))
}

func TestTOMLDecodeCompatibility(t *testing.T) {
	type cfg struct {
		Enabled bool  `json:"enabled"`
		Ports   []int `json:"ports"`
		Inline  struct {
			Retry struct {
				Count int `json:"count"`
			} `json:"retry"`
		} `json:"inline"`
	}

	text := []byte(`enabled = true
ports = [80, 443]
inline = { retry.count = 3 }
`)

	var out cfg
	if err := zc.NewTOML(text).Decode(&out, "json"); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !out.Enabled {
		t.Fatalf("enabled = false")
	}
	if !reflect.DeepEqual(out.Ports, []int{80, 443}) {
		t.Fatalf("ports = %#v", out.Ports)
	}
	if out.Inline.Retry.Count != 3 {
		t.Fatalf("retry.count = %d", out.Inline.Retry.Count)
	}
}

func TestTOMLDuplicateKeysReturnError(t *testing.T) {
	rmap := map[string]any{}
	err := zc.ParseTOML([]byte("name = \"a\"\nname = \"b\"\n"), rmap)
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("unexpected error: %v", err)
	}
}
