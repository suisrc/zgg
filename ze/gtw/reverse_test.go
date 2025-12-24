// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gtw_test

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/suisrc/zgg/ze/gtw"
)

// go test -v ze/pxy/reverse_test.go -run Test_proxy

func Test_proxy(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:8080")
	proxy := gtw.NewSingleProxy(target)
	proxy.ServeHTTP(nil, nil) // next
}

// go test -v ze/pxy/reverse_test.go -run Test_lower

func Test_lower(t *testing.T) {
	str := "123ABCabcDEF"
	buf := bytes.NewBuffer([]byte{})
	for _, r := range str {
		buf.WriteByte(gtw.LowerByte(byte(r)))
	}
	t.Log(buf.String())

}
