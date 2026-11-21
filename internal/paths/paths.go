// Package paths gathers the on-disk locations Argus uses.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Paths are the Argus directories for one user (Home) and one tmux server.
type Paths struct {
	Home  string // the user's home directory
	State string // volatile state: session statuses, indexes, metrics
	Share string // installed files (the hook)
	Conf  string // configuration
}

// Default uses the current user's home directory.
func Default(socket string) Paths {
	h, err := os.UserHomeDir()
	if err != nil {
		h = "/"
	}
	return For(h, socket)
}

// For builds the paths for a home directory. A test tmux server (any socket
// other than "argus") gets its own state directory, so it never touches the
// state of the real server's sessions.
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
func (p Paths) Snapshot() string     { return filepath.Join(p.State, "_sessions.json") }
func (p Paths) StatusLine() string   { return filepath.Join(p.Share, "statusline.sh") }
func (p Paths) Usage() string        { return filepath.Join(p.State, "_usage") }
func (p Paths) Templates() string    { return filepath.Join(p.Conf, "templates.toml") }
func (p Paths) TmuxEvent() string    { return filepath.Join(p.State, "_tmux") }

// Opencode is where the opencode plugin writes each conversation's status.
func (p Paths) Opencode() string { return filepath.Join(p.State, "_opencode") }

// Status is the status file of one session.
func (p Paths) Status(id string) string { return filepath.Join(p.State, id) }

// OpencodePlugins is opencode's plugin directory.
func (p Paths) OpencodePlugins() string {
	return filepath.Join(p.Home, ".config", "opencode", "plugins")
}

// OpencodeDB is opencode's conversation database.
func (p Paths) OpencodeDB() string {
	return filepath.Join(p.Home, ".local", "share", "opencode", "opencode.db")
}

// ClaudeProjects holds Claude Code's transcripts.
func (p Paths) ClaudeProjects() string { return filepath.Join(p.Home, ".claude", "projects") }

// CodexHooks is Codex's hook file (CODEX_HOME overrides ~/.codex).
func (p Paths) CodexHooks() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return filepath.Join(h, "hooks.json")
	}
	return filepath.Join(p.Home, ".codex", "hooks.json")
}

// GeminiSettings is the Gemini CLI user settings file.
func (p Paths) GeminiSettings() string { return filepath.Join(p.Home, ".gemini", "settings.json") }

// Expand replaces a leading "~" with the home directory.
func (p Paths) Expand(s string) string {
	if s == "~" {
		return p.Home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(p.Home, s[2:])
	}
	return s
}

// Pretty abbreviates the home directory as "~" for display.
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
