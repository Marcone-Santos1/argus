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
	pn.selectKey("a:" + pn.agents[1].ID)
	pn.showSelected()
	if got, want := layout(), "panel: editor:"+d2+" agent:"+d2; got != want {
		t.Fatalf("other folder brings its editor:\n got %s\nwant %s", got, want)
	}
	pn.hideSession()
	if got, want := layout(), "panel: editor:"+d2; got != want {
		t.Fatalf("session hidden, code stays:\n got %s\nwant %s", got, want)
	}
	pn.selectKey("a:" + pn.agents[0].ID)
	pn.showSelected()
	if got, want := layout(), "panel: editor:"+d1+" agent:"+d1; got != want {
		t.Fatalf("session back beside the code:\n got %s\nwant %s", got, want)
	}
	pn.toggleCode()
	if got, want := layout(), "panel: agent:"+d1; got != want {
		t.Fatalf("code closed:\n got %s\nwant %s", got, want)
	}
	if len(pn.editors) != 2 {
		t.Fatalf("editors keep running hidden: %v", pn.editors)
	}
}
