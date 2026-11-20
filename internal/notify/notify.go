// Package notify mostra avisos na Central de Notificações do macOS.
package notify

import (
	"os/exec"
)

// script recebe título e texto como argumentos: nada do texto vira código.
const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"
end run`

// Command monta o comando do aviso (separado para teste).
func Command(title, body string) *exec.Cmd {
	return exec.Command("/usr/bin/osascript", "-e", script, title, body)
}

// Send mostra o aviso sem esperar (um processo por aviso).
func Send(title, body string) {
	cmd := Command(title, body)
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}
