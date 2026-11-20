// Package paths reúne os caminhos que o Argus usa no disco.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths são os diretórios do Argus para um usuário (Home) e um servidor tmux.
type Paths struct {
	Home  string // diretório do usuário
	State string // estado volátil: status das sessões, índices, métricas
	Share string // arquivos instalados (hook)
	Conf  string // configuração
}

// Default usa o diretório do usuário atual.
func Default(socket string) Paths {
	h, err := os.UserHomeDir()
	if err != nil {
		h = "/"
	}
	return For(h, socket)
}

// For monta os caminhos para um home. Um servidor tmux de teste (socket diferente
// de "argus") ganha um diretório de estado próprio, para nunca mexer no estado
// das sessões do servidor de verdade.
func For(home, socket string) Paths {
	state := filepath.Join(home, ".local", "state", "argus")
	if socket != "" && socket != "argus" {
		state += "-" + socket
	}
	return Paths{
		Home:  home,
		State: state,
		Share: filepath.Join(home, ".local", "share", "argus"),
		Conf:  filepath.Join(home, ".config", "argus"),
	}
}

func (p Paths) Hook() string         { return filepath.Join(p.Share, "hook.sh") }
func (p Paths) TmuxConf() string     { return filepath.Join(p.Conf, "tmux.conf") }
func (p Paths) Config() string       { return filepath.Join(p.Conf, "config.toml") }
func (p Paths) Prefs() string        { return filepath.Join(p.State, "_prefs.json") }
func (p Paths) HistoryCache() string { return filepath.Join(p.State, "_history.json") }
func (p Paths) Metrics() string      { return filepath.Join(p.State, "_metrics") }
func (p Paths) TmuxEvent() string    { return filepath.Join(p.State, "_tmux") }

// Opencode é onde o plugin do opencode grava o status de cada conversa.
func (p Paths) Opencode() string { return filepath.Join(p.State, "_opencode") }

// Status é o arquivo de status de uma sessão.
func (p Paths) Status(id string) string { return filepath.Join(p.State, id) }

// OpencodePlugins é a pasta de plugins do opencode.
func (p Paths) OpencodePlugins() string {
	return filepath.Join(p.Home, ".config", "opencode", "plugins")
}

// OpencodeDB é o banco de conversas do opencode.
func (p Paths) OpencodeDB() string {
	return filepath.Join(p.Home, ".local", "share", "opencode", "opencode.db")
}

// ClaudeProjects guarda as transcrições do Claude Code.
func (p Paths) ClaudeProjects() string { return filepath.Join(p.Home, ".claude", "projects") }

// Expand troca um "~" inicial pelo home.
func (p Paths) Expand(s string) string {
	if s == "~" {
		return p.Home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(p.Home, s[2:])
	}
	return s
}

// Pretty abrevia o home como "~" para exibição.
func (p Paths) Pretty(path string) string {
	rel, err := filepath.Rel(p.Home, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	if rel == "." {
		return "~"
	}
	return "~/" + rel
}
