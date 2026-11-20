package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "argus"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// launch garante o servidor e o painel vivos e substitui este processo pelo
// cliente tmux: fora do tmux não sobra nenhum processo do Argus.
func launch() error {
	if _, err := os.Stat(tmuxConfPath()); err != nil {
		return errors.New("rode `argus install` primeiro")
	}
	if t := os.Getenv("TMUX"); strings.Contains(t, "/"+socketName+",") {
		return errors.New("você já está dentro do Argus (Ctrl+\\ volta para a lista)")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	self := selfPath()
	if !sessionExists() {
		_, err := tm("-f", tmuxConfPath(),
			"new-session", "-d", "-s", sessionName, "-n", mainWindow, self, "panel", ";",
			"set", "-w", "-t", sessionName+":"+mainWindow, "remain-on-exit", "on")
		if err != nil {
			return err
		}
	} else if err := ensurePanel(self); err != nil {
		return err
	}
	// A janela sempre acompanha o tamanho do terminal (um resize manual a deixaria presa).
	_, _ = tm("set", "-w", "-u", "-t", sessionName+":"+mainWindow, "window-size", ";",
		"select-window", "-t", sessionName+":"+mainWindow, ";", "select-pane", "-t", sessionName+":"+mainWindow+".0")
	return syscall.Exec(tmuxBin, []string{"tmux", "-L", socketName, "attach-session", "-t", sessionName}, os.Environ())
}

func ensurePanel(self string) error {
	ps, err := listPanes()
	if err != nil {
		return err
	}
	target := ""
	for _, p := range ps {
		if p.Role == "panel" {
			if !p.Dead {
				return nil
			}
			target = p.ID
		}
	}
	if target == "" {
		for _, p := range ps {
			if p.inMain() && p.Index == 0 {
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
	_, err = tm("respawn-pane", "-k", "-t", target, self, "panel")
	return err
}
