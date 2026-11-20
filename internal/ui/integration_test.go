package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// Integration with a real tmux server: create, show, hide and close sessions.
// The "agent" is a sleep, and the panel is just some tmux pane.
func TestSessionsWithRealTmux(t *testing.T) {
	tc := tmux.New(fmt.Sprintf("argus-ui-%d", os.Getpid()))
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
	cfg, err := config.Parse(`
[[profile]]
name = "fake"
group = "PERSONAL"
command = "sleep 300"
`, home)
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

	mainPanes := func() []tmux.Pane {
		ps, err := tc.ListPanes()
		if err != nil {
			t.Fatal(err)
		}
		var m []tmux.Pane
		for _, x := range ps {
			if x.InMain() {
				m = append(m, x)
			}
		}
		return m
	}

	pn.reconcile()
	if m := mainPanes(); len(m) != 2 || m[1].Role != tmux.RolePlaceholder {
		t.Fatalf("with no sessions, the right shows the empty space: %+v", m)
	}

	dir := t.TempDir()
	pn.createAgent(dir, cfg.Profiles[0], nil, "")
	if len(pn.agents) != 1 || !pn.agents[0].Displayed {
		t.Fatalf("session created and shown: %+v (msg %q)", pn.agents, pn.msg)
	}
	if m := mainPanes(); len(m) != 2 || m[1].Role != tmux.RoleAgent {
		t.Fatalf("the session takes the right side: %+v", m)
	}

	pn.createAgent(dir, cfg.Profiles[0], nil, "")
	if len(pn.agents) != 2 || pn.agents[1].Name == pn.agents[0].Name {
		t.Fatalf("a second session in the same folder gets another name: %+v", pn.agents)
	}

	pn.hideSession()
	for _, a := range pn.agents {
		if a.Displayed {
			t.Fatalf("Esc hides: %+v", pn.agents)
		}
	}
	if m := mainPanes(); m[1].Role != tmux.RolePlaceholder || len(pn.agents) != 2 {
		t.Fatalf("hidden but alive: %+v", m)
	}

	pn.sel = pn.nthAgentItem(0)
	if !pn.showSelected() || !pn.current().Displayed {
		t.Fatal("show again")
	}
	pn.killSelected()
	if pn.mode == modeConfirm { // idle: no confirmation
		t.Fatal("an idle session does not ask for confirmation")
	}
	if len(pn.agents) != 1 {
		t.Fatalf("closed: %+v", pn.agents)
	}
	if m := mainPanes(); len(m) != 2 || m[1].Role != tmux.RolePlaceholder {
		t.Fatalf("after closing the visible one, the empty space is back: %+v", m)
	}

	// Switch the remaining session to another profile: same pane, same
	// conversation, new command with --resume.
	cfg2, err := config.Parse(`
[[profile]]
name = "one"
command = "sh -c 'exec sleep 300' sh"
[[profile]]
name = "two"
command = "sh -c 'exec sleep 301' sh"
`, home)
	if err != nil {
		t.Fatal(err)
	}
	pn.d.Config = cfg2
	a := pn.agents[0]
	_ = tc.SetPaneOptions(a.Pane, "@argus_profile", "one")
	if err := os.WriteFile(p.Status(a.ID), []byte("done\nconv-123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pn.reconcile()
	pn.sel = pn.nthAgentItem(0)
	pn.switchAccount()
	if pn.mode != modePickProfile || len(pn.profChoices) != 1 || pn.profChoices[0].Name != "two" {
		t.Fatalf("offers the other profile: mode=%v %+v (msg %q)", pn.mode, pn.profChoices, pn.msg)
	}
	pn.handleKey("enter")
	ps, _ := tc.ListPanes()
	var moved tmux.Pane
	for _, x := range ps {
		if x.AgentID == a.ID {
			moved = x
		}
	}
	if moved.ID != a.Pane || moved.Profile != "two" || moved.Session != "conv-123" {
		t.Fatalf("same pane, new profile, same conversation: %+v", moved)
	}
	start, _ := tc.Run("display", "-p", "-t", a.Pane, "#{pane_start_command}")
	if !strings.Contains(start, "--resume") || !strings.Contains(start, "conv-123") || !strings.Contains(start, "sleep 301") {
		t.Fatalf("restarted with the new profile and --resume: %q", start)
	}
}
