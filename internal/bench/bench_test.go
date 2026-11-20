package bench

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseCPUTime(t *testing.T) {
	cases := map[string]time.Duration{
		"0:00.42":       420 * time.Millisecond,
		" 1:02.50\n":    62500 * time.Millisecond,
		"1:00:00.00":    time.Hour,
		"2-00:00:01.00": 48*time.Hour + time.Second,
	}
	for in, want := range cases {
		if got := ParseCPUTime(in); got != want {
			t.Errorf("ParseCPUTime(%q) = %v, quer %v", in, got, want)
		}
	}
}

func TestPrint(t *testing.T) {
	var b bytes.Buffer
	Print(&b, []Row{{"a", "< 1", "0,5", ok(true)}, {"b", "< 1", "2", ok(false)}, {"c", "", "info", nil}})
	out := b.String()
	if !strings.Contains(out, "✓ 0,5") || !strings.Contains(out, "✗ 2") || strings.Contains(out, "✓ info") {
		t.Fatalf("relatório:\n%s", out)
	}
}
