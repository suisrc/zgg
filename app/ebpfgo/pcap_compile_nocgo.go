//go:build ignore
// +build ignore

//-go:build !cgo
//- +build !cgo

// pcap_compile_nocgo.go 对应的适配文件

package ebpfgo

import "fmt"

func compilePcapRulesWithLibpcap(rules string) ([]pcapRuleInsn, error) {
	return nil, fmt.Errorf("pcap-rules requires cgo and libpcap; current build was compiled with CGO_ENABLED=0")
}
