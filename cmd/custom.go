package cmd

import (
	"fmt"

	"github.com/suisrc/zgg/z"
)

func init() {
	z.CMDR["hello"] = hello
}

func hello() {
	fmt.Println("hello world!")
}
