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

func TestParseMeminfo(t *testing.T) {
	used, total, ok := parseMeminfo("MemTotal:       16000 kB\nMemFree:  100 kB\nMemAvailable:    4000 kB\n")
	if !ok || total != 16000*1024 || used != 12000*1024 {
		t.Fatalf("used=%d total=%d ok=%v", used, total, ok)
	}
	if _, _, ok := parseMeminfo("MemTotal: 10 kB\n"); ok {
		t.Fatal("needs MemAvailable")
	}
}
