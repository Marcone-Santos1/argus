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
	p.firstPrompt = "leia o ONDE-PARAMOS.md"
	_, cmd := p.launch("a1", prof, nil)
	if !strings.Contains(cmd[len(cmd)-1], "claude 'leia o ONDE-PARAMOS.md'") {
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
	if !strings.Contains(foot, "per 5h ▰▰▰▱▱ 64% ") {
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
