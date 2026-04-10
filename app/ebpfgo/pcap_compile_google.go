//go:build ignore
// +build ignore

// google 纯 Go 实现的 pcap 规则编译器， 不依赖 cgo 和 libpcap.

package ebpfgo

import (
	"fmt"

	"github.com/google/gopacket/pcap"
)

const (
	pcapLinkTypeEthernet = 1
	pcapSnapLen          = 65535
)

func compilePcapRulesWithLibpcap(rules string) ([]pcapRuleInsn, error) {
	instructions, err := pcap.CompileBPFFilter(pcapLinkTypeEthernet, pcapSnapLen, rules)
	if err != nil {
		return nil, err
	}
	if len(instructions) > pcapRulesMaxInsns {
		return nil, fmt.Errorf("pcap instruction count %d invalid", len(instructions))
	}
	insns := make([]pcapRuleInsn, len(instructions))
	for i, insn := range instructions {
		insns[i] = pcapRuleInsn{
			Code: insn.Code,
			JT:   insn.Jt,
			JF:   insn.Jf,
			K:    insn.K,
		}
	}
	return insns, nil
}
