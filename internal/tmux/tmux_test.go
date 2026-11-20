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
		"/p/api", "api", "1795000000000000000", "ses_x", "/p/repo"}, "\t")
	ps := ParsePanes(line + "\nlinha quebrada\n")
	if len(ps) != 1 {
		t.Fatalf("esperava 1 painel: %+v", ps)
	}
	p := ps[0]
	if p.ID != "%3" || !p.InMain() || p.Index != 1 || p.Dead || p.PID != 4242 || p.Role != RoleAgent ||
		p.AgentID != "ab12cd" || p.Profile != "claude-work" || p.Dir != "/p/api" || p.Name != "api" ||
		p.Created != 1795000000000000000 || p.Session != "ses_x" || p.Worktree != "/p/repo" {
		t.Fatalf("campos: %+v", p)
	}
}

// Teste de integração com um servidor tmux de verdade, num socket próprio.
func TestServer(t *testing.T) {
	if _, err := exec.LookPath(New("x").Bin); err != nil {
		t.Skip("tmux não instalado")
	}
	c := New(fmt.Sprintf("argus-test-%d", os.Getpid()))
	t.Cleanup(func() { _, _ = c.Run("kill-server") })

	if c.HasSession() {
		t.Fatal("servidor novo não deveria ter sessão")
	}
	if _, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", Session, "-n", MainWindow, "sleep", "30"); err != nil {
		t.Fatal(err)
	}
	if !c.HasSession() {
		t.Fatal("sessão deveria existir")
	}
	ps, err := c.ListPanes()
	if err != nil || len(ps) != 1 {
		t.Fatalf("painéis: %v %+v", err, ps)
	}
	if err := c.SetPaneOptions(ps[0].ID, "@argus_role", RoleAgent, "@argus_name", "nome com espaço"); err != nil {
		t.Fatal(err)
	}
	ps, _ = c.ListPanes()
	if ps[0].Role != RoleAgent || ps[0].Name != "nome com espaço" || !ps[0].InMain() {
		t.Fatalf("opções gravadas: %+v", ps[0])
	}
	if _, err := c.Run("comando-que-nao-existe"); err == nil {
		t.Fatal("comando inválido deveria dar erro")
	}
}
