package cmd_test

import "testing"

// go test -v cmd/custom_test.go -run Test_name

func Test_name(t *testing.T) {
	t.Log("hello world!")
}
