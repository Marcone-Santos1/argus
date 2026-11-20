// Comando argus: painel de terminal para rodar várias sessões de agentes
// (Claude Code, opencode) em paralelo, sobre um servidor tmux próprio.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Marcone-Santos1/argus/internal/bench"
	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/install"
	"github.com/Marcone-Santos1/argus/internal/notify"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/sysmon"
	"github.com/Marcone-Santos1/argus/internal/tmux"
	"github.com/Marcone-Santos1/argus/internal/ui"
)

var started = time.Now()

const usage = `argus: painel para várias sessões de Claude Code e opencode

uso:
  argus              abre o painel (cria o servidor tmux se preciso)
  argus install      instala hooks, plugin do opencode, config do tmux e config padrão
  argus uninstall    remove tudo o que o install colocou
  argus bench [-s N] mede o desempenho (N segundos por medição de CPU, padrão 60)
`

func main() {
	socket := os.Getenv("ARGUS_SOCKET")
	if socket == "" {
		socket = "argus"
	}
	p := paths.Default(socket)
	t := tmux.New(socket)

	var err error
	if len(os.Args) < 2 {
		err = launch(p, t)
	} else {
		switch os.Args[1] {
		case "panel":
			err = runPanel(p, t)
		case "install":
			err = install.Install(p, os.Stdout)
			if err == nil && t.HasSession() {
				if _, e := t.Run("source-file", p.TmuxConf()); e == nil {
					fmt.Println("tmux:            config recarregada no servidor em uso")
				}
			}
		case "uninstall":
			err = install.Uninstall(p, os.Stdout)
			if err == nil && t.HasSession() {
				fmt.Println("o servidor do Argus continua com suas sessões; feche com `tmux -L argus kill-server`")
			}
		case "bench":
			fs := flag.NewFlagSet("bench", flag.ExitOnError)
			secs := fs.Int("s", 60, "segundos por medição de CPU")
			_ = fs.Parse(os.Args[2:])
			err = bench.Run(p, t, *secs, os.Stdout)
		case "-h", "--help", "help":
			fmt.Print(usage)
		default:
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "argus:", err)
		os.Exit(1)
	}
}

func runPanel(p paths.Paths, t *tmux.Client) error {
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		return err
	}
	return ui.New(ui.Deps{
		Paths: p, Config: cfg, Tmux: t, Self: os.Getenv("TMUX_PANE"),
		Notify: notify.Send, Monitor: sysmon.New(), Started: started,
	}).Run()
}

func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "argus"
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// launch garante o servidor e o painel vivos e troca este processo pelo cliente
// tmux: fora do tmux não sobra nenhum processo do Argus.
func launch(p paths.Paths, t *tmux.Client) error {
	if _, err := os.Stat(p.TmuxConf()); err != nil {
		return errors.New("rode `argus install` primeiro")
	}
	if strings.Contains(os.Getenv("TMUX"), "/"+t.Socket+",") {
		return errors.New("você já está dentro do Argus (Ctrl+\\ volta para a lista)")
	}
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		return err
	}
	self := selfPath()
	main := tmux.Session + ":" + tmux.MainWindow
	if !t.HasSession() {
		args := []string{"-f", p.TmuxConf(), "new-session", "-d", "-s", tmux.Session, "-n", tmux.MainWindow}
		if t.Socket != "argus" {
			args = append(args, "-e", "ARGUS_SOCKET="+t.Socket)
		}
		args = append(args, self, "panel", ";", "set", "-w", "-t", main, "remain-on-exit", "on")
		if _, err := t.Run(args...); err != nil {
			return err
		}
	} else if err := ensurePanel(t, self); err != nil {
		return err
	}
	// A janela sempre acompanha o tamanho do terminal (um resize manual a deixaria presa).
	_, _ = t.Run("set", "-w", "-u", "-t", main, "window-size", ";",
		"select-window", "-t", main, ";", "select-pane", "-t", main+".0")
	return syscall.Exec(t.Bin, []string{"tmux", "-L", t.Socket, "attach-session", "-t", tmux.Session}, os.Environ())
}

func ensurePanel(t *tmux.Client, self string) error {
	ps, err := t.ListPanes()
	if err != nil {
		return err
	}
	target := ""
	for _, p := range ps {
		if p.Role == tmux.RolePanel {
			if !p.Dead {
				return nil
			}
			target = p.ID
		}
	}
	if target == "" {
		for _, p := range ps {
			if p.InMain() && p.Index == 0 {
				if !p.Dead && p.Role == "" {
					return nil // painel subindo
				}
				target = p.ID
			}
		}
	}
	if target == "" {
		return errors.New("não achei o painel na sessão do tmux; rode `tmux -L argus kill-server` e abra de novo")
	}
	_, err = t.Run("respawn-pane", "-k", "-t", target, self, "panel")
	return err
}
