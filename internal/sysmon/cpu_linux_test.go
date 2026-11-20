//go:build linux

package sysmon

import "testing"

func TestParseProcStat(t *testing.T) {
	busy, total, ok := parseProcStat("cpu  100 5 50 800 20 3 2 0 0 0")
	if !ok || total != 980 || busy != 160 {
		t.Fatalf("busy=%d total=%d ok=%v", busy, total, ok)
	}
	if _, _, ok := parseProcStat("cpu0 1 2 3 4"); ok {
		t.Fatal("only the aggregate line counts")
	}
	if _, ok := nextPID(); !ok {
		t.Skip("ns_last_pid not readable here")
	}
}
