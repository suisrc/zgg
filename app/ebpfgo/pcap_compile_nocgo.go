//go:build !cgo
// +build !cgo

package ebpfgo

import "fmt"

func compilePcapRulesWithLibpcap(rules string) ([]pcapRuleInsn, error) {
	return nil, fmt.Errorf("pcap-rules requires cgo and libpcap; current build was compiled with CGO_ENABLED=0")
}
