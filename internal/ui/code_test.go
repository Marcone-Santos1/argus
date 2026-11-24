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

// The code column with a real tmux server: the "editor" and the "agent" are sleeps.
func TestCodeColumnWithRealTmux(t *testing.T) {
	tc := tmux.New(fmt.Sprintf("argus-code-%d", os.Getpid()))
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
editor = "sleep 301"
[[profile]]
name = "fake"
command = "sleep 300"
`, home)
	if err != nil {
		t.Fatal(err)
	}
	self, err := tc.Run("-f", "/dev/null", "new-session", "-d", "-s", tmux.Session, "-n", tmux.MainWindow,
		"-x", "160", "-y", "40", "-P", "-F", "#{pane_id}", "sleep", "300", ";",
		"set", "-w", "-t", tmux.Session+":"+tmux.MainWindow, "remain-on-exit", "on")
	if err != nil {
		t.Fatal(err)
	}
	_ = tc.SetPaneOptions(self, "@argus_role", tmux.RolePanel)
	pn := New(Deps{Paths: p, Config: cfg, Tmux: tc, Self: self})
	layout := func() string {
		out, err := tc.Run("list-panes", "-t", tmux.Session+":"+tmux.MainWindow, "-F", "#{@argus_role}:#{@argus_dir}")
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(out, "\n", " ")
	}
	pn.reconcile()
	d1, d2 := t.TempDir(), t.TempDir()
	pn.createAgent(d1, cfg.Profiles[0], nil, "")
	pn.createAgent(d2, cfg.Profiles[0], nil, "")
	pn.selectKey("a:" + pn.agents[0].ID)
	pn.focusSession()

	pn.toggleCode()
	if got, want := layout(), "panel: editor:"+d1+" agent:"+d1; got != want {
		t.Fatalf("code opened:\n got %s\nwant %s (msg %q)", got, want, pn.msg)
	}
	// The code column stays with the session it was opened in.
	pn.selectKey("a:" + pn.agents[1].ID)
	pn.showSelected()
	if got, want := layout(), "panel: agent:"+d2; got != want {
		t.Fatalf("another session: no code column:\n got %s\nwant %s", got, want)
	}
	pn.toggleCode()
	if got, want := layout(), "panel: editor:"+d2+" agent:"+d2; got != want {
		t.Fatalf("its own code column:\n got %s\nwant %s", got, want)
	}
	pn.hideSession()
	if got, want := layout(), "panel: editor:"+d2; got != want {
		t.Fatalf("session hidden, code stays:\n got %s\nwant %s", got, want)
	}
	pn.selectKey("a:" + pn.agents[0].ID)
	pn.showSelected()
	if got, want := layout(), "panel: editor:"+d1+" agent:"+d1; got != want {
		t.Fatalf("back to the first session, its code column again:\n got %s\nwant %s", got, want)
	}
	pn.toggleCode()
	if got, want := layout(), "panel: agent:"+d1; got != want {
		t.Fatalf("code closed:\n got %s\nwant %s", got, want)
	}
	pn.selectKey("a:" + pn.agents[1].ID)
	pn.showSelected()
	if got, want := layout(), "panel: editor:"+d2+" agent:"+d2; got != want {
		t.Fatalf("the second keeps its column:\n got %s\nwant %s", got, want)
	}
	pn.toggleCode()
	pn.selectKey("a:" + pn.agents[0].ID)
	pn.showSelected()
	if len(pn.editors) != 2 {
		t.Fatalf("editors keep running hidden: %v", pn.editors)
	}
	// "Code only" from the new-session picker: the folder's editor, no agent.
	d3 := t.TempDir()
	pn.openCodeOnly(d3)
	if got, want := layout(), "panel: editor:"+d3+" agent:"+d1; got != want {
		t.Fatalf("code only:\n got %s\nwant %s (msg %q)", got, want, pn.msg)
	}
	if len(pn.agents) != 2 {
		t.Fatalf("no agent started: %d", len(pn.agents))
	}
}

func TestBrowseOffersCreate(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(home+"/Code/desafio", 0o755); err != nil {
		t.Fatal(err)
	}
	got := browseDirs("~/Code/desafios-resolvidos", home, 10)
	if len(got) != 1 || !got[0].Create || got[0].Path != home+"/Code/desafios-resolvidos" {
		t.Fatalf("create offer: %+v", got)
	}
	got = browseDirs("~/Code/desa", home, 10)
	if len(got) != 2 || got[0].Rel != "~/Code/desafio/" || !got[1].Create {
		t.Fatalf("match then create: %+v", got)
	}
	if got := browseDirs("~/Code/desafio/", home, 10); len(got) != 1 || got[0].Create {
		t.Fatalf("an existing folder is not offered for creation: %+v", got)
	}
}

func TestIsFresh(t *testing.T) {
	for cmd, want := range map[string]bool{"fresh": true, "fresh .": true, "/opt/homebrew/bin/fresh": true, "fresh -a x": false, "micro": false, "": false} {
		if isFresh(cmd) != want {
			t.Errorf("isFresh(%q) = %v", cmd, !want)
		}
	}
	if a, b := freshName("/p/a"), freshName("/p/b"); a == b || len(a) != 14 {
		t.Fatalf("names: %q %q", a, b)
	}
}
