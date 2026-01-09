package cmd

import (
	"fmt"

	"github.com/suisrc/zgg/z"
)

func init() {
	z.CMD["hello"] = hello
}

func hello() {
	fmt.Println("hello world!")
}
