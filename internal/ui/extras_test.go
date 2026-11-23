package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/gitinfo"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/usage"
)

func TestLaunchFirstPrompt(t *testing.T) {
	p := testPanel(t)
	prof := p.d.Config.Profiles[0]
	p.firstPrompt = "leia o NOTES.md"
	_, cmd := p.launch("a1", prof, nil)
	if !strings.Contains(cmd[len(cmd)-1], "claude 'leia o NOTES.md'") {
		t.Fatalf("first message: %q", cmd[len(cmd)-1])
	}
	if p.firstPrompt != "" {
		t.Fatal("the first message is used once")
	}
	_, cmd = p.launch("a2", prof, nil)
	if strings.Contains(cmd[len(cmd)-1], "ONDE") {
		t.Fatal("the next session starts without it")
	}
	oc, _ := p.d.Config.Profile("opencode")
	p.firstPrompt = "oi"
	if _, cmd = p.launch("a3", oc, nil); !strings.HasSuffix(cmd[len(cmd)-1], "opencode '--prompt' 'oi'") {
		t.Fatalf("opencode: %q", cmd[len(cmd)-1])
	}
	p.firstPrompt = "oi"
	if _, cmd = p.launch("a4", prof, &history.Entry{Tool: "claude", ID: "abc"}); strings.Contains(cmd[len(cmd)-1], "oi") {
		t.Fatal("a resumed conversation gets no first message")
	}
}

func TestSessionExtrasFit(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	a := agent{ID: "a1", Name: "api", Dir: "/p/api", Profile: "personal", Status: status.Done, Since: now}
	p.git["/p/api"] = gitState{gitinfo.Info{Branch: "feature/um-branch-comprido", Changed: 12, Ahead: 3}, true}
	p.reply["a1"] = strings.Repeat("Corrigi o teste e subi o commit. ", 5)
	p.usage["a1"] = usage.Info{Context: 91, FiveHour: &usage.Limit{Percent: 64, Resets: now.Add(time.Hour)}, At: now}
	p.agents = []agent{a}
	for _, w := range []int{30, 60, 120} {
		rows := p.agentRows(a, 1, false, now, w)
		if len(rows) != 2 {
			t.Fatalf("done session: title and last reply: %d rows", len(rows))
		}
		for _, l := range append(rows, p.groupHeader(a.Dir, true, w)) {
			if width(visible(l)) != w {
				t.Fatalf("w=%d: %q", w, visible(l))
			}
		}
	}
	got := visible(strings.Join(append(p.agentRows(a, 1, false, now, 160), p.groupHeader(a.Dir, false, 160)), "\n"))
	for _, want := range []string{"⎇ feature/um-branch-comprido ✎12 ↑3", "ctx ▰▰▰▰▰ 91%", "↳ Corrigi o teste", "per"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	a.Status = status.Working
	if rows := visible(strings.Join(p.agentRows(a, 1, false, now, 60), "")); strings.Contains(rows, "Corrigi") || !strings.Contains(rows, "ctx") {
		t.Fatalf("the last reply hides while it works, the context shows: %q", rows)
	}
	foot := ""
	for _, l := range p.statsLines(80) {
		foot += visible(l)
	}
	if !strings.Contains(foot, "per 5h ▰▰▰▰▱│▱ 64%") {
		t.Fatalf("account usage in the footer: %q", foot)
	}
}

func TestTemplatesKeys(t *testing.T) {
	p := testPanel(t)
	p.d.Config.Templates = []config.Template{{Name: "conf", Dir: "/p", Profile: "personal"}}
	p.handleKey("t")
	if p.mode != modeTemplates || p.tplCount() != 1 {
		t.Fatalf("t opens the templates: %v %d", p.mode, p.tplCount())
	}
	p.handleKey("x")
	if p.mode != modeTemplates || len(p.d.Config.Templates) != 1 {
		t.Fatal("a config.toml template is not deleted from the panel")
	}
	p.w, p.h = 60, 20
	if v := visible(strings.Join(p.viewTemplates(), "\n")); !strings.Contains(v, "conf") {
		t.Fatalf("view: %q", v)
	}
	p.handleKey("esc")
	if p.mode != modeList {
		t.Fatal("esc closes")
	}
}

func TestUntilText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		35 * time.Minute: "35m", 2*time.Hour + 48*time.Minute: "2h48",
		4*24*time.Hour + 6*time.Hour: "4d6h", -time.Minute: "0m",
	} {
		if got := untilText(d); got != want {
			t.Errorf("untilText(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestBackgroundShellsKeepItRunning(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	a := agent{ID: "a1", Name: "api", Dir: "/p/api", Profile: "personal", Status: status.Done, Since: now}
	p.agents = []agent{a}
	p.reply["a1"] = "Tests are running; the commit is ready."
	p.shells["a1"] = 1
	rows := p.agentRows(a, 1, false, now, 60)
	got := visible(strings.Join(rows, "\n"))
	if strings.Contains(got, "✓") || !strings.Contains(got, "1 ") || !strings.Contains(rows[0], cWorking) {
		t.Fatalf("a done session with a background shell still shows as running: %q", got)
	}
	if h := p.panelTitle(); strings.Contains(h, "✓") {
		t.Fatalf("counted as done in the title: %q", h)
	}
}

func TestHeavySessionsAndDisk(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Profile: "personal", Status: status.Working, Since: now},
		{ID: "a2", Dir: "/p/web", Profile: "personal", Status: status.Working, Since: now},
	}
	p.rebuildItems("")
	p.res["a1"] = sessRes{CPU: 240, hot: hotTicks}
	p.res["a2"] = sessRes{CPU: 180, hot: hotTicks - 1}
	if h := p.heavyWarning(); h != "" {
		t.Fatalf("one heavy session on a calm machine is no warning: %q", h)
	}
	p.res["a2"] = sessRes{CPU: 180, hot: hotTicks}
	p.cpuSeries["a1"] = []float64{100, 200, 240}
	if h := p.heavyWarning(); !strings.Contains(h, "api 240% + web 180%") {
		t.Fatalf("two heavy sessions: %q", h)
	}
	if got := visible(strings.Join(p.agentRows(p.agents[0], 1, false, now, 60), "")); !strings.Contains(got, "cpu ▄▇█ 240%") {
		t.Fatalf("the session shows its CPU: %q", got)
	}
	p.reading.DiskFree = 2 << 30
	foot := visible(strings.Join(p.footer(), "\n"))
	if !strings.Contains(foot, "2.0G") || !strings.Contains(foot, "⚠") {
		t.Fatalf("low disk warning: %q", foot)
	}
	for b, want := range map[uint64]string{27 << 30: "27G", 4600 << 20: "4.5G", 820 << 20: "820M"} {
		if got := size(b); got != want {
			t.Errorf("size(%d) = %q, want %q", b, got, want)
		}
	}
}

func TestPace(t *testing.T) {
	// Half the limit used in the first hour of five: it runs out in an hour.
	if d, ok := paceEnds(50, 0.2, 5*time.Hour); !ok || d != time.Hour {
		t.Fatalf("paceEnds: %v %v", d, ok)
	}
	if _, ok := paceEnds(50, 0.05, 5*time.Hour); ok {
		t.Fatal("too early in the window to guess")
	}
	p := testPanel(t)
	now := time.Now()
	p.agents = []agent{{ID: "a1", Profile: "personal"}}
	p.usage["a1"] = usage.Info{FiveHour: &usage.Limit{Percent: 50, Resets: now.Add(4 * time.Hour)}, At: now}
	foot := visible(strings.Join(p.statsLines(80), ""))
	if !strings.Contains(foot, "▰│▰▰▱▱▱ 50% ⚠1h00") {
		t.Fatalf("pace marker and warning: %q", foot)
	}
}

func TestScreenTail(t *testing.T) {
	rule := strings.Repeat("─", 40)
	screen := []string{"● Done.", "", "", "  details", rule, "❯ ", rule, "  ⏵⏵ auto mode on", "", ""}
	got := screenTail(screen, 10)
	if strings.Join(got, "|") != "● Done.||  details" {
		t.Fatalf("screenTail: %q", got)
	}
}

func TestTimelineRows(t *testing.T) {
	base := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	ev := []stateEvent{
		{ID: "a", Name: "api", State: status.Working, At: base.Unix()},
		{ID: "a", State: status.Waiting, At: base.Add(30 * time.Minute).Unix()},
		{ID: "a", State: status.Working, At: base.Add(40 * time.Minute).Unix()},
		{ID: "b", Name: "idle", State: status.Idle, At: base.Unix()},
		{ID: "a", State: stClosed, At: base.Add(time.Hour).Unix()},
	}
	rows := timelineRows(ev, base.Add(2*time.Hour), map[string]bool{"b": true})
	if len(rows) != 1 || rows[0].name != "api" || rows[0].waited != 10*time.Minute {
		t.Fatalf("rows: %+v", rows)
	}
	if st := rows[0].stateAt(base.Add(31*time.Minute), base.Add(35*time.Minute)); st != status.Waiting {
		t.Fatalf("stateAt: %q", st)
	}
	if st := rows[0].stateAt(base.Add(70*time.Minute), base.Add(80*time.Minute)); st != "" {
		t.Fatalf("after closing: %q", st)
	}
}

func TestOverviewFits(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Profile: "personal", Status: status.Working, Since: now},
		{ID: "a2", Dir: "/p/web", Profile: "personal", Status: status.Waiting, Since: now},
		{ID: "a3", Dir: "/p/web", Profile: "personal", Status: status.Done, Since: now},
	}
	p.rebuildItems("")
	p.screens["a2"] = []string{"Do you want to proceed?", "❯ 1. Yes", "  2. No"}
	for _, size := range [][2]int{{60, 20}, {130, 30}, {204, 54}} {
		p.w, p.h = size[0], size[1]
		out := p.viewOverview()
		if len(out) != p.h {
			t.Fatalf("%v: %d lines", size, len(out))
		}
		for _, l := range out {
			if width(visible(l)) > p.w {
				t.Fatalf("%v: line too wide (%d): %q", size, width(visible(l)), visible(l))
			}
		}
		if all := visible(strings.Join(out, "\n")); !strings.Contains(all, "1. Yes") {
			t.Fatalf("%v: the waiting session's question shows", size)
		}
	}
	// Two columns at 130: the second tile is to the right of the first.
	p.w, p.h = 130, 30
	p.viewOverview()
	p.clickTile(0, p.ov.tileW+p.ov.gap+3)
	if p.ovSel != 1 {
		t.Fatalf("click selects the tile: %d", p.ovSel)
	}
}
