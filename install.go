package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

//go:embed assets/hook.sh
var hookScript string

//go:embed assets/tmux.conf
var tmuxConf string

// Evento do Claude Code -> argumento do hook.
var claudeEvents = []struct{ Event, Arg string }{
	{"SessionStart", "start"},
	{"UserPromptSubmit", "prompt"},
	{"PermissionRequest", "perm"},
	{"Notification", "notify"},
	{"PostToolUse", "tool"},
	{"Stop", "stop"},
	{"SessionEnd", "end"},
}

// Pastas de config do Claude usadas pelos perfis (padrão ~/.claude).
func claudeConfigDirs(cfg *Config) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, p := range cfg.Profiles {
		if p.Label() != "claude" {
			continue
		}
		d := filepath.Join(homeDir(), ".claude")
		if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
			d = expand(v)
		}
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func install() error {
	for _, d := range []string{stateDir, shareDir, confDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	fill := func(s string) string { return strings.ReplaceAll(s, "__STATE_DIR__", stateDir) }
	if err := os.WriteFile(hookPath(), []byte(fill(hookScript)), 0o755); err != nil {
		return err
	}
	fmt.Println("hook:        ", hookPath())
	if err := os.WriteFile(tmuxConfPath(), []byte(fill(tmuxConf)), 0o644); err != nil {
		return err
	}
	fmt.Println("tmux.conf:   ", tmuxConfPath())
	if !exists(configPath()) {
		if err := os.WriteFile(configPath(), []byte(defaultConfig), 0o644); err != nil {
			return err
		}
		fmt.Println("config:      ", configPath(), "(novo)")
	} else {
		fmt.Println("config:      ", configPath(), "(mantido)")
	}
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config.toml inválido: %w", err)
	}
	for _, d := range claudeConfigDirs(cfg) {
		msg, err := addClaudeHooks(filepath.Join(d, "settings.json"))
		if err != nil {
			return err
		}
		fmt.Println("hooks Claude:", filepath.Join(d, "settings.json"), msg)
	}
	if sessionExists() {
		if _, err := tm("source-file", tmuxConfPath()); err == nil {
			fmt.Println("tmux:         config recarregada no servidor em uso")
		}
	}
	return nil
}

func hookEntry(arg string) string {
	return fmt.Sprintf(`{"matcher":"","hooks":[{"type":"command","command":%q}]}`, hookPath()+" "+arg)
}

func addClaudeHooks(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = []byte("{}")
	} else if err != nil {
		return "", err
	}
	if !gjson.ValidBytes(data) {
		return "", fmt.Errorf("%s não é JSON válido; nada foi alterado", path)
	}
	js := string(data)
	added := 0
	for _, e := range claudeEvents {
		key := "hooks." + e.Event
		if strings.Contains(gjson.Get(js, key).Raw, hookPath()) {
			continue
		}
		if gjson.Get(js, key).IsArray() {
			js, err = sjson.SetRaw(js, key+".-1", hookEntry(e.Arg))
		} else {
			js, err = sjson.SetRaw(js, key, "["+hookEntry(e.Arg)+"]")
		}
		if err != nil {
			return "", err
		}
		added++
	}
	if added == 0 {
		return "(já instalados)", nil
	}
	if err := writeWithBackup(path, data, js); err != nil {
		return "", err
	}
	return fmt.Sprintf("(+%d eventos; backup ao lado)", added), nil
}

func writeWithBackup(path string, old []byte, js string) error {
	// Escreve no arquivo real (settings.json pode ser link) e mantém a permissão.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if len(old) > 2 {
		bak := fmt.Sprintf("%s.argus-bak-%d", path, time.Now().Unix())
		if err := os.WriteFile(bak, old, 0o600); err != nil {
			return err
		}
	}
	out := pretty.PrettyOptions([]byte(js), &pretty.Options{Indent: "  ", Width: 120})
	tmp := path + ".argus-tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return err
	}
	_ = os.Chmod(tmp, mode)
	return os.Rename(tmp, path)
}

func removeClaudeHooks(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil
	}
	js := string(data)
	removed := 0
	for _, e := range claudeEvents {
		key := "hooks." + e.Event
		arr := gjson.Get(js, key)
		if !arr.IsArray() {
			continue
		}
		items := arr.Array()
		for i := len(items) - 1; i >= 0; i-- {
			if strings.Contains(items[i].Raw, hookPath()) {
				js, err = sjson.Delete(js, fmt.Sprintf("%s.%d", key, i))
				if err != nil {
					return removed, err
				}
				removed++
			}
		}
		if len(gjson.Get(js, key).Array()) == 0 {
			js, _ = sjson.Delete(js, key)
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, writeWithBackup(path, data, js)
}

func uninstall() error {
	cfg, err := loadConfig()
	if err != nil {
		cfg = &Config{Profiles: []Profile{{Command: "claude"}, {Command: "claude", Env: map[string]string{"CLAUDE_CONFIG_DIR": "~/.claude-work"}}}}
	}
	for _, d := range claudeConfigDirs(cfg) {
		p := filepath.Join(d, "settings.json")
		n, err := removeClaudeHooks(p)
		if err != nil {
			return err
		}
		fmt.Printf("hooks Claude: %s (-%d)\n", p, n)
	}
	if sessionExists() {
		fmt.Println("tmux:         o servidor do Argus continua rodando com suas sessões; feche com `tmux -L argus kill-server`")
	}
	for _, d := range []string{shareDir, confDir, stateDir} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
		fmt.Println("removido:    ", d)
	}
	fmt.Println("o binário fica em", selfPath(), "(apague se quiser)")
	return nil
}
