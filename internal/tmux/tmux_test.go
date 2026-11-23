package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParsePanes(t *testing.T) {
	line := strings.Join([]string{"%3", "main", "1", "0", "4242", "agent", "ab12cd", "claude-work",
		"/p/api", "api", "1795000000000000000", "ses_x", "/p/repo", "152", "54", "0"}, "\t")
	ps := ParsePanes(line + "\nbroken line\n")
	if len(ps) != 1 {
		t.Fatalf("expected 1 pane: %+v", ps)
	}
	p := ps[0]
	if p.ID != "%3" || !p.InMain() || p.Index != 1 || p.Dead || p.PID != 4242 || p.Role != RoleAgent ||
		p.AgentID != "ab12cd" || p.Profile != "claude-work" || p.Dir != "/p/api" || p.Name != "api" ||
		p.Created != 1795000000000000000 || p.Session != "ses_x" || p.Worktree != "/p/repo" ||
		p.Width != 152 || p.Height != 54 || p.Zoomed {
		t.Fatalf("fields: %+v", p)
	}
}

// Integration test against a real tmux server on its own socket.
func TestServer(t *testing.T) {
	if _, err := exec.LookPath(New("x").Bin); err != nil {
		t.Skip("tmux not installed")
	}
	c := New(fmt.Sprintf("argus-test-%d", os.Getpid()))
	t.Cleanup(func() { _, _ = c.Run("kill-server") })

	if c.HasSession() {
		t.Fatal("a new server has no session")
	}
	if _, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", Session, "-n", MainWindow, "sleep", "30"); err != nil {
		t.Fatal(err)
	}
	if !c.HasSession() {
		t.Fatal("the session should exist")
	}
	ps, err := c.ListPanes()
	if err != nil || len(ps) != 1 {
		t.Fatalf("panes: %v %+v", err, ps)
	}
	if err := c.SetPaneOptions(ps[0].ID, "@argus_role", RoleAgent, "@argus_name", "name with space"); err != nil {
		t.Fatal(err)
	}
	ps, _ = c.ListPanes()
	if ps[0].Role != RoleAgent || ps[0].Name != "name with space" || !ps[0].InMain() {
		t.Fatalf("stored options: %+v", ps[0])
	}
	if _, err := c.Run("no-such-command"); err == nil {
		t.Fatal("an invalid command must fail")
	}
}
