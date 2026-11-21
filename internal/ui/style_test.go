package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/status"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visible(s string) string { return ansi.ReplaceAllString(s, "") }

func TestLineHasExactWidth(t *testing.T) {
	for _, w := range []int{5, 20, 62} {
		got := visible(line(w, bgSel, sp("abc", cAccent), sp(strings.Repeat("x", 100), "")))
		if width(got) != w {
			t.Fatalf("largura %d: %q (%d)", w, got, width(got))
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("texto cortado termina em …: %q", got)
		}
	}
	short := line(10, bgWait, sp("ab", ""))
	if width(visible(short)) != 10 || !strings.Contains(short, bgWait) {
		t.Fatalf("completa com o fundo: %q", short)
	}
}

func TestRowsFitTheWidth(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	pr := p.d.Config.Profiles[0]
	for _, w := range []int{30, 44, 62, 120} {
		for _, st := range []string{status.Idle, status.Working, status.Waiting, status.Done} {
			for _, sel := range []bool{false, true} {
				a := agent{Name: "um nome bem comprido para cortar no meio", Dir: "/p/api", Profile: pr.Name, Status: st, Since: now.Add(-time.Minute)}
				if got := visible(p.agentRow(a, 1, sel, false, now, w)); width(got) != w {
					t.Fatalf("sessão w=%d %s sel=%v: %d colunas: %q", w, st, sel, width(got), got)
				}
			}
		}
		e := history.Entry{Tool: "claude", Title: strings.Repeat("título ", 30), Updated: now.Add(-time.Hour)}
		if got := visible(p.recentRow(e, true, true, now, w)); width(got) != w {
			t.Fatalf("conversa w=%d: %d colunas", w, width(got))
		}
		if got := visible(p.header(w)); width(got) != w {
			t.Fatalf("cabeçalho w=%d: %d colunas", w, width(got))
		}
	}
}

func TestHeaderCountsByStatus(t *testing.T) {
	p := testPanel(t)
	p.agents = []agent{{Status: status.Working}, {Status: status.Working}, {Status: status.Waiting}}
	h := visible(p.header(80))
	if !strings.Contains(h, "● 2") || !strings.Contains(h, "◐ 1") || strings.Contains(h, "✓") {
		t.Fatalf("contadores: %q", h)
	}
}

func TestWaitingRowStandsOut(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	a := agent{Name: "api", Dir: "/p/api", Profile: p.d.Config.Profiles[0].Name, Status: status.Waiting}
	if !strings.Contains(p.agentRow(a, 1, false, true, now, 60), bgWait) {
		t.Fatal("sessão esperando você ganha fundo âmbar")
	}
	if !strings.Contains(p.agentRow(a, 1, true, true, now, 60), bgSel) {
		t.Fatal("a seleção tem prioridade sobre o âmbar")
	}
}

func TestSpark(t *testing.T) {
	if got := spark([]float64{0, 50, 100, 150, -5}); got != "▁▅██▁" {
		t.Fatalf("spark: %q", got)
	}
}

func TestIsTemp(t *testing.T) {
	for dir, want := range map[string]bool{
		"/private/tmp/x": true, "/private/tmp": true, "/var/folders/ab/T/y": true,
		"/Users/ana/Projects": false, "/tmpdir": false,
	} {
		if isTemp(dir) != want {
			t.Errorf("isTemp(%q) != %v", dir, want)
		}
	}
}
