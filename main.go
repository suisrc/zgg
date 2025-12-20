package main

import (
	_ "github.com/suisrc/zgg/app"
	_ "github.com/suisrc/zgg/cmd"
	"github.com/suisrc/zgg/z"
	_ "github.com/suisrc/zgg/ze/rdx"
)

/**
 * 程序入口
 */
func main() {
	z.Execute()
}
