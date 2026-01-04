// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

package zc_test

import (
	"log"
	"testing"

	"github.com/suisrc/zgg/z/zc"
)

// go test -v z/zc/log_test.go -run Test_log0

func Test_log0(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("test")
}

// go test -v z/zc/log_test.go -run Test_log1

func Test_log1(t *testing.T) {
	zc.Println("test")
}
