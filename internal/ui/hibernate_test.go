package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

func TestCanHibernate(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	old := now.Add(-3 * time.Hour)
	base := agent{ID: "a1", Profile: "personal", Session: "conv-1", Status: status.Done, Since: old}
	if !p.canHibernate(base, now, 2*time.Hour) {
		t.Fatal("finished, out of sight, 3h untouched: hibernates")
	}
	cases := map[string]func(a *agent){
		"too recent":   func(a *agent) { a.Since = now.Add(-time.Hour) },
		"working":      func(a *agent) { a.Status = status.Working },
		"waiting":      func(a *agent) { a.Status = status.Waiting },
		"on screen":    func(a *agent) { a.Displayed = true },
		"no id":        func(a *agent) { a.Session = "" },
		"subagent":     func(a *agent) { a.Subagents = 1 },
		"not claude":   func(a *agent) { a.Profile = "opencode" },
		"already done": func(a *agent) { a.Asleep = old },
	}
	for name, change := range cases {
		a := base
		change(&a)
		if p.canHibernate(a, now, 2*time.Hour) {
			t.Errorf("%s: must not hibernate", name)
		}
	}
	p.seenAt["a1"] = now.Add(-30 * time.Minute)
	if p.canHibernate(base, now, 2*time.Hour) {
		t.Error("looked at 30 min ago: must not hibernate")
	}
	delete(p.seenAt, "a1")
	p.shells["a1"] = 1
	if p.canHibernate(base, now, 2*time.Hour) {
		t.Error("a shell still running behind it: must not hibernate")
	}
	delete(p.shells, "a1")
	if p.canHibernate(base, now, 0) {
		t.Error("0 turns it off")
	}
	for in, want := range map[string]time.Duration{"2h": 2 * time.Hour, "90m": 90 * time.Minute, "0": 0, "": 0, "x": 0} {
		c := config.Config{HibernateAfter: in}
		if got := c.Hibernate(); got != want {
			t.Errorf("Hibernate(%q) = %v, want %v", in, got, want)
		}
	}
	if cfg, _ := config.Parse("", "/h"); cfg.Hibernate() != 2*time.Hour {
		t.Error("default: 2h")
	}
}

// With a real tmux server: the process closes, the pane stays in the list,
// and Enter resumes the same conversation in it.
func TestHibernateWithRealTmux(t *testing.T) {
	tc := tmux.New(fmt.Sprintf("argus-hib-%d", os.Getpid()))
	if _, err := exec.LookPath(tc.Bin); err != nil {
		t.Skip("tmux not installed")
	}
	t.Cleanup(func() { _, _ = tc.Run("kill-server") })
	t.Setenv("SHELL", "/bin/sh")

	home := t.TempDir()
	p := paths.For(home, tc.Socket)
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args")
	fake := filepath.Join(bin, "claude") // Label() is "claude"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\" >> "+argsFile+"\nexec sleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse("[[profile]]\nname = \"me\"\ncommand = \""+fake+"\"\n", home)
	if err != nil {
		t.Fatal(err)
	}
	self, err := tc.Run("-f", "/dev/null", "new-session", "-d", "-s", tmux.Session, "-n", tmux.MainWindow,
		"-x", "120", "-y", "40", "-P", "-F", "#{pane_id}", "sleep", "300", ";",
		"set", "-w", "-t", tmux.Session+":"+tmux.MainWindow, "remain-on-exit", "on")
	if err != nil {
		t.Fatal(err)
	}
	_ = tc.SetPaneOptions(self, "@argus_role", tmux.RolePanel)
	pn := New(Deps{Paths: p, Config: cfg, Tmux: tc, Self: self})
	pn.reconcile()
	pn.createAgent(t.TempDir(), cfg.Profiles[0], nil, "")
	pn.hideSession()
	if len(pn.agents) != 1 {
		t.Fatalf("one session: %+v", pn.agents)
	}
	a := pn.agents[0]
	_ = tc.SetPaneOptions(a.Pane, "@argus_session", "conv-1")
	st := p.Status(a.ID)
	_ = status.Write(st, status.Done)
	old := time.Now().Add(-3 * time.Hour)
	_ = os.Chtimes(st, old, old)
	pn.reconcile()

	pn.hibernateIdle(time.Now())
	a = pn.agents[0]
	if a.Asleep.IsZero() || a.Status != stAsleep || len(pn.agents) != 1 {
		t.Fatalf("hibernated and still listed: %+v", pn.agents)
	}
	if out, _ := tc.Run("display", "-p", "-t", a.Pane, "#{pane_dead}"); out != "1" {
		t.Fatalf("its process is closed: dead=%q", out)
	}
	pn.reconcile() // the dead pane is not taken for a closed session
	if len(pn.agents) != 1 || pn.agents[0].Asleep.IsZero() {
		t.Fatalf("kept across reconcile: %+v", pn.agents)
	}
	if rows := visible(strings.Join(pn.agentRows(pn.agents[0], 1, false, time.Now(), 60), "")); !strings.Contains(rows, "◌") {
		t.Fatalf("shown as asleep: %q", rows)
	}

	pn.sel = pn.nthAgentItem(0)
	pn.focusSession()
	a = pn.agents[0]
	if !a.Asleep.IsZero() || a.Status == stAsleep || !a.Displayed {
		t.Fatalf("Enter wakes it and shows it: %+v", a)
	}
	var got []byte
	for i := 0; i < 50; i++ {
		if got, _ = os.ReadFile(argsFile); strings.Contains(string(got), "--resume conv-1") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(string(got), "--resume conv-1") {
		t.Fatalf("resumes the same conversation: %q", got)
	}
	if out, _ := tc.Run("display", "-p", "-t", a.Pane, "#{pane_dead}[#{@argus_sleep}]"); out != "0[]" {
		t.Fatalf("running again, mark cleared: %q", out)
	}
}
