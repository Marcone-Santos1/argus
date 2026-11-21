package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/sysmon"
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
				for _, l := range p.agentRows(a, 1, sel, now, w) {
					if got := visible(l); width(got) != w {
						t.Fatalf("sessão w=%d %s sel=%v: %d colunas: %q", w, st, sel, width(got), got)
					}
				}
			}
		}
		e := history.Entry{Tool: "claude", Title: strings.Repeat("título ", 30), Updated: now.Add(-time.Hour)}
		if got := visible(p.entryRow(e, true, now, w)); width(got) != w {
			t.Fatalf("conversa w=%d: %d colunas", w, width(got))
		}
		f := item{dir: "/p/um-projeto-de-nome-comprido", agent: -1, folder: true, count: 12, entry: e}
		if got := visible(p.folderRow(f, true, now, w)); width(got) != w {
			t.Fatalf("pasta w=%d: %d colunas: %q", w, width(got), got)
		}
	}
}

func TestHeaderCountsByStatus(t *testing.T) {
	p := testPanel(t)
	p.agents = []agent{{Status: status.Working}, {Status: status.Working}, {Status: status.Waiting}}
	h := p.panelTitle()
	if !strings.Contains(h, "● 2") || !strings.Contains(h, "◐ 1") || strings.Contains(h, "✓") {
		t.Fatalf("contadores: %q", h)
	}
}

func TestWaitingRowStandsOut(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	a := agent{Name: "api", Dir: "/p/api", Profile: p.d.Config.Profiles[0].Name, Status: status.Waiting}
	if !strings.Contains(p.agentRows(a, 1, false, now, 60)[0], bgWait) {
		t.Fatal("sessão esperando você ganha fundo âmbar")
	}
	if !strings.Contains(p.agentRows(a, 1, true, now, 60)[0], bgSel) {
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

func TestPromptLinesFit(t *testing.T) {
	p := testPanel(t)
	pr := prompt{Context: strings.Repeat("touch x ", 30), Question: "Do you want to proceed?",
		Options: []string{"Yes", "Yes, and don't ask again for these commands", "No"}, Numbered: true}
	p.agents = []agent{{ID: "a1", Name: "api", Dir: "/p/api", Status: status.Waiting}}
	p.prompts["a1"] = pr
	p.answering = "a1"
	for _, w := range []int{30, 62, 120} {
		for _, l := range append(p.promptLines(pr, w), p.answerLines(w)...) {
			if width(visible(l)) != w {
				t.Fatalf("w=%d: %d columns: %q", w, width(visible(l)), visible(l))
			}
		}
	}
	got := visible(strings.Join(p.promptLines(pr, 80), "\n"))
	if !strings.Contains(got, "1 Yes") || !strings.Contains(got, "3 No") {
		t.Fatalf("options in the preview: %q", got)
	}
}

func TestStatsWrapInsteadOfCutting(t *testing.T) {
	p := testPanel(t)
	p.reading = sysmon.Reading{CPU: 34, MemUsed: 12 << 30, MemTotal: 16 << 30, PerMinute: 49892}
	p.cpuHist = []float64{30, 40, 34}
	for _, w := range []int{30, 45, 120} {
		all := ""
		lines := p.statsLines(w)
		for _, l := range lines {
			if width(visible(l)) != w {
				t.Fatalf("w=%d: %q", w, visible(l))
			}
			all += visible(l)
		}
		if !strings.Contains(all, "49892") || strings.Contains(all, "…") {
			t.Fatalf("w=%d: proc/min cut: %q", w, all)
		}
		if w == 120 && len(lines) != 1 {
			t.Fatalf("wide: one line, got %d", len(lines))
		}
	}
}
