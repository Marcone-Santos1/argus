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
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// newServer starts a tmux server with a stand-in panel and returns a panel
// bound to it.
func newServer(t *testing.T, socket string, p paths.Paths, cfg *config.Config) (*Panel, *tmux.Client) {
	t.Helper()
	tc := tmux.New(socket)
	self, err := tc.Run("-f", "/dev/null", "new-session", "-d", "-s", tmux.Session, "-n", tmux.MainWindow,
		"-x", "120", "-y", "40", "-P", "-F", "#{pane_id}", "sleep", "300", ";",
		"set", "-w", "-t", tmux.Session+":"+tmux.MainWindow, "remain-on-exit", "on")
	if err != nil {
		t.Fatal(err)
	}
	_ = tc.SetPaneOptions(self, "@argus_role", tmux.RolePanel)
	return New(Deps{Paths: p, Config: cfg, Tmux: tc, Self: self}), tc
}

func needTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(tmux.New("x").Bin); err != nil {
		t.Skip("tmux not installed")
	}
	t.Setenv("SHELL", "/bin/sh")
}

// Sessions open when the server dies come back, with their names, on the
// next start; declining the offer forgets them.
func TestRestoreAfterServerDies(t *testing.T) {
	needTmux(t)
	home := t.TempDir()
	sock := fmt.Sprintf("argus-restore-%d", os.Getpid())
	p := paths.For(home, sock)
	_ = os.MkdirAll(p.State, 0o755)
	cfg, _ := config.Parse("[[profile]]\nname = \"fake\"\ngroup = \"PERSONAL\"\ncommand = \"sleep 300\"\n", home)
	dirA, dirB := t.TempDir(), t.TempDir()

	pn, tc := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc.Run("kill-server") })
	pn.reconcile()
	pn.offerRestore() // nothing to offer yet: as Run does at start
	pn.createAgent(dirA, cfg.Profiles[0], nil, "")
	pn.createAgent(dirB, cfg.Profiles[0], nil, "")
	_ = tc.SetPaneOptions(pn.agents[1].Pane, "@argus_name", "renamed")
	pn.reconcile()
	if snap := loadSnapshot(p.Snapshot(), cfg); len(snap) != 2 {
		t.Fatalf("snapshot after creating two sessions: %+v", snap)
	}

	// The machine restarts: the server and its sessions are gone.
	_, _ = tc.Run("kill-server")
	pn2, tc2 := newServer(t, sock, p, cfg)
	pn2.reconcile()
	pn2.offerRestore()
	if pn2.mode != modeConfirm || len(pn2.restoreOffer) != 2 {
		t.Fatalf("restore offered: mode %v offer %+v", pn2.mode, pn2.restoreOffer)
	}
	pn2.reconcile() // the panel keeps working meanwhile; the offer is not erased
	if snap := loadSnapshot(p.Snapshot(), cfg); len(snap) != 2 {
		t.Fatal("the snapshot survives while the offer is pending")
	}
	fn := pn2.confirmFn
	pn2.mode, pn2.confirmFn, pn2.confirmNo = modeList, nil, nil
	fn()
	if len(pn2.agents) != 2 {
		t.Fatalf("restored sessions: %+v (msg %q)", pn2.agents, pn2.msg)
	}
	names := pn2.agents[0].Name + "," + pn2.agents[1].Name
	if !strings.Contains(names, "renamed") {
		t.Errorf("names come back too: %s", names)
	}

	// Declining: the next start offers nothing.
	_, _ = tc2.Run("kill-server")
	pn3, tc3 := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc3.Run("kill-server") })
	pn3.reconcile()
	pn3.offerRestore()
	pn3.handleKey("n")
	if pn3.restoreOffer != nil || len(loadSnapshot(p.Snapshot(), cfg)) != 0 {
		t.Fatal("declining forgets the sessions")
	}
}

// A session waiting on a numbered prompt is answered from the list: the
// option's number reaches the session.
func TestAnswerFromList(t *testing.T) {
	needTmux(t)
	home := t.TempDir()
	sock := fmt.Sprintf("argus-answer-%d", os.Getpid())
	p := paths.For(home, sock)
	_ = os.MkdirAll(p.State, 0o755)
	out := filepath.Join(t.TempDir(), "answer")
	script := filepath.Join(t.TempDir(), "agent.sh")
	_ = os.WriteFile(script, []byte(`printf ' Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n Esc to cancel\n'
stty raw -echo; c=$(dd bs=1 count=1 2>/dev/null); stty sane
printf '%s' "$c" > `+out+`
sleep 300
`), 0o755)
	cfg, _ := config.Parse("[[profile]]\nname = \"fake\"\ngroup = \"PERSONAL\"\ncommand = \"sh "+script+"\"\n", home)
	pn, tc := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc.Run("kill-server") })
	pn.createAgent(t.TempDir(), cfg.Profiles[0], nil, "")
	time.Sleep(300 * time.Millisecond)

	a := pn.agents[0]
	_ = status.Write(p.Status(a.ID), status.Waiting)
	pn.refreshStatus(0, false)
	pn.capturePrompt(a.ID)
	pr, ok := pn.prompts[a.ID]
	if !ok || len(pr.Options) != 2 || pr.Question != "Do you want to proceed?" {
		t.Fatalf("prompt read from the session's screen: %+v", pr)
	}
	pn.sel = pn.nthAgentItem(0)
	pn.handleKey("y")
	if pn.mode != modeAnswer {
		t.Fatalf("y opens the answer (msg %q)", pn.msg)
	}
	pn.handleKey("2")
	time.Sleep(300 * time.Millisecond)
	if b, _ := os.ReadFile(out); string(b) != "2" {
		t.Fatalf("the session received %q, want 2", b)
	}
	if pn.mode != modeList {
		t.Fatal("back to the list after answering")
	}
}

// A hibernated session comes back hibernated after the server dies: no
// process until Enter resumes it.
func TestRestoreKeepsHibernated(t *testing.T) {
	needTmux(t)
	home := t.TempDir()
	sock := fmt.Sprintf("argus-restore-sleep-%d", os.Getpid())
	p := paths.For(home, sock)
	_ = os.MkdirAll(p.State, 0o755)
	bin := filepath.Join(t.TempDir(), "claude") // the label decides it is Claude
	_ = os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755)
	cfg, _ := config.Parse("[[profile]]\nname = \"fake\"\ngroup = \"PERSONAL\"\ncommand = \""+bin+"\"\n", home)
	const conv = "11111111-2222-3333-4444-555555555555"

	pn, tc := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc.Run("kill-server") })
	pn.reconcile()
	pn.offerRestore()
	pn.createAgent(t.TempDir(), cfg.Profiles[0], &history.Entry{Tool: history.Claude, ID: conv}, "")
	pn.createAgent(t.TempDir(), cfg.Profiles[0], nil, "")
	pn.reconcile()
	for i := range pn.agents {
		if pn.agents[i].Session == conv {
			if !pn.hibernate(&pn.agents[i], time.Now()) {
				t.Fatal("hibernate")
			}
		}
	}
	pn.reconcile()

	_, _ = tc.Run("kill-server")
	pn2, tc2 := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc2.Run("kill-server") })
	pn2.reconcile()
	pn2.offerRestore()
	fn := pn2.confirmFn
	pn2.mode, pn2.confirmFn, pn2.confirmNo = modeList, nil, nil
	fn()
	time.Sleep(1500 * time.Millisecond) // the note's shell exits
	pn2.reconcile()
	asleep, awake := 0, 0
	for _, a := range pn2.agents {
		if a.Asleep.IsZero() {
			awake++
		} else {
			asleep++
			if a.Session != conv {
				t.Errorf("the hibernated one keeps its conversation: %q", a.Session)
			}
		}
	}
	if asleep != 1 || awake != 1 {
		t.Fatalf("restored: %d asleep, %d awake (%+v)", asleep, awake, pn2.agents)
	}
	if !pn2.current().Asleep.IsZero() {
		t.Fatal("the awake session is the one selected")
	}
}
