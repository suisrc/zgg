package ebpfgo

import "testing"

func TestCompilePcapRules(t *testing.T) {
	insns, err := compilePcapRules("udp")
	if err != nil {
		t.Fatalf("compilePcapRules returned error: %v", err)
	}
	if len(insns) == 0 {
		t.Fatal("compilePcapRules returned no instructions")
	}
	if len(insns) > pcapRulesMaxInsns {
		t.Fatalf("compilePcapRules returned %d instructions, limit is %d", len(insns), pcapRulesMaxInsns)
	}
}

func TestCompilePcapRulesEmpty(t *testing.T) {
	insns, err := compilePcapRules("   ")
	if err != nil {
		t.Fatalf("compilePcapRules returned error for empty rules: %v", err)
	}
	if len(insns) != 0 {
		t.Fatalf("compilePcapRules returned %d instructions for empty rules", len(insns))
	}
}

func TestCompilePcapRulesInvalid(t *testing.T) {
	if _, err := compilePcapRules("host and and"); err == nil {
		t.Fatal("compilePcapRules should reject invalid expressions")
	}
}
