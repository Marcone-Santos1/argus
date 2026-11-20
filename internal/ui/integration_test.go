package ui

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// Integração com um servidor tmux de verdade: criar, mostrar, esconder e
// encerrar sessões. O "agente" é um sleep, e o painel é um painel tmux qualquer.
func TestSessionsWithRealTmux(t *testing.T) {
	tc := tmux.New(fmt.Sprintf("argus-ui-%d", os.Getpid()))
	if _, err := exec.LookPath(tc.Bin); err != nil {
		t.Skip("tmux não instalado")
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
group = "PESSOAL"
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
		t.Fatalf("sem sessões, a direita mostra o espaço vazio: %+v", m)
	}

	dir := t.TempDir()
	pn.createAgent(dir, cfg.Profiles[0], nil, "")
	if len(pn.agents) != 1 || !pn.agents[0].Displayed {
		t.Fatalf("sessão criada e mostrada: %+v (msg %q)", pn.agents, pn.msg)
	}
	if m := mainPanes(); len(m) != 2 || m[1].Role != tmux.RoleAgent {
		t.Fatalf("a sessão ocupa a direita: %+v", m)
	}

	pn.createAgent(dir, cfg.Profiles[0], nil, "")
	if len(pn.agents) != 2 || pn.agents[1].Name == pn.agents[0].Name {
		t.Fatalf("segunda sessão na mesma pasta ganha outro nome: %+v", pn.agents)
	}

	pn.hideSession()
	for _, a := range pn.agents {
		if a.Displayed {
			t.Fatalf("Esc esconde: %+v", pn.agents)
		}
	}
	if m := mainPanes(); m[1].Role != tmux.RolePlaceholder || len(pn.agents) != 2 {
		t.Fatalf("escondida, mas viva: %+v", m)
	}

	pn.sel = pn.nthAgentItem(0)
	if !pn.showSelected() || !pn.current().Displayed {
		t.Fatal("mostrar de novo")
	}
	pn.killSelected()
	if pn.mode == modeConfirm { // ociosa: não pede confirmação
		t.Fatal("sessão ociosa não pede confirmação")
	}
	if len(pn.agents) != 1 {
		t.Fatalf("encerrada: %+v", pn.agents)
	}
	if m := mainPanes(); len(m) != 2 || m[1].Role != tmux.RolePlaceholder {
		t.Fatalf("depois de encerrar a visível, volta o espaço vazio: %+v", m)
	}
}
