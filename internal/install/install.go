// Package install coloca e remove o que o Argus precisa fora do próprio
// binário: o hook de status do Claude Code, o plugin do opencode e a
// configuração do tmux.
package install

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
)

//go:embed assets/hook.sh
var hookScript string

//go:embed assets/tmux.conf
var tmuxConf string

//go:embed assets/argus-status.ts
var opencodePlugin string

// PluginFile é o nome do plugin dentro da pasta de plugins do opencode.
const PluginFile = "argus-status.ts"

// ClaudeEvents liga cada evento do Claude Code ao argumento do hook.
var ClaudeEvents = []struct{ Event, Arg string }{
	{"SessionStart", "start"},
	{"UserPromptSubmit", "prompt"},
	{"PermissionRequest", "perm"},
	{"Notification", "notify"},
	{"PostToolUse", "tool"},
	{"Stop", "stop"},
	{"SessionEnd", "end"},
}

// HookScript é o hook pronto para um diretório de estado.
func HookScript(stateDir string) string {
	return strings.ReplaceAll(hookScript, "__STATE_DIR__", stateDir)
}

// TmuxConf é a configuração do tmux pronta para um diretório de estado.
func TmuxConf(stateDir string) string {
	return strings.ReplaceAll(tmuxConf, "__STATE_DIR__", stateDir)
}

// OpencodePlugin é o plugin pronto para uma pasta de estados do opencode.
func OpencodePlugin(dir string) string {
	return strings.ReplaceAll(opencodePlugin, "__OPENCODE_DIR__", dir)
}

// ClaudeSettingsFiles são os settings.json das contas do Claude configuradas.
func ClaudeSettingsFiles(cfg *config.Config, home string) []string {
	seen := map[string]bool{}
	var files []string
	for _, p := range cfg.ProfilesFor("claude") {
		f := filepath.Join(p.ClaudeConfigDir(home), "settings.json")
		real := f
		if r, err := filepath.EvalSymlinks(f); err == nil {
			real = r
		}
		if !seen[real] {
			seen[real] = true
			files = append(files, f)
		}
	}
	return files
}

// Install escreve hook, plugin e config do tmux e liga o hook nos settings do Claude.
func Install(p paths.Paths, out io.Writer) error {
	for _, d := range []string{p.State, p.Share, p.Conf, p.Opencode()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(p.Hook(), []byte(HookScript(p.State)), 0o755); err != nil {
		return err
	}
	fmt.Fprintln(out, "hook:          ", p.Hook())
	if err := os.WriteFile(p.TmuxConf(), []byte(TmuxConf(p.State)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "tmux.conf:     ", p.TmuxConf())
	if _, err := os.Stat(p.Config()); err != nil {
		if err := os.WriteFile(p.Config(), []byte(config.DefaultTOML), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "config:        ", p.Config(), "(novo)")
	} else {
		fmt.Fprintln(out, "config:        ", p.Config(), "(mantido)")
	}
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		return fmt.Errorf("config.toml inválido: %w", err)
	}
	for _, f := range ClaudeSettingsFiles(cfg, p.Home) {
		msg, err := addHooksFile(f, p.Hook())
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "hooks Claude:  ", f, msg)
	}
	if len(cfg.ProfilesFor("opencode")) > 0 {
		dir := p.OpencodePlugins()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		f := filepath.Join(dir, PluginFile)
		if err := os.WriteFile(f, []byte(OpencodePlugin(p.Opencode())), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "plugin opencode:", f)
	}
	return nil
}

// Uninstall desfaz o Install.
func Uninstall(p paths.Paths, out io.Writer) error {
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		cfg, _ = config.Parse(config.DefaultTOML, p.Home)
	}
	for _, f := range ClaudeSettingsFiles(cfg, p.Home) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		js, n, err := RemoveHooks(data, p.Hook())
		if err != nil {
			return err
		}
		if n > 0 {
			if err := writeWithBackup(f, data, js); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "hooks Claude:   %s (-%d)\n", f, n)
	}
	plugin := filepath.Join(p.OpencodePlugins(), PluginFile)
	if err := os.Remove(plugin); err == nil {
		fmt.Fprintln(out, "removido:      ", plugin)
	}
	for _, d := range []string{p.Share, p.Conf, p.State} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
		fmt.Fprintln(out, "removido:      ", d)
	}
	return nil
}

func hookEntry(command string) string {
	return fmt.Sprintf(`{"matcher":"","hooks":[{"type":"command","command":%q}]}`, command)
}

// AddHooks acrescenta o hook do Argus aos eventos do Claude Code, sem mexer nos
// hooks que já existem. Devolve o JSON novo e quantos eventos ganharam o hook.
func AddHooks(data []byte, hook string) ([]byte, int, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}
	if !gjson.ValidBytes(data) {
		return nil, 0, fmt.Errorf("settings.json não é JSON válido")
	}
	js := string(data)
	added := 0
	for _, e := range ClaudeEvents {
		key := "hooks." + e.Event
		cur := gjson.Get(js, key)
		if strings.Contains(cur.Raw, hook) {
			continue
		}
		var err error
		if cur.IsArray() {
			js, err = sjson.SetRaw(js, key+".-1", hookEntry(hook+" "+e.Arg))
		} else {
			js, err = sjson.SetRaw(js, key, "["+hookEntry(hook+" "+e.Arg)+"]")
		}
		if err != nil {
			return nil, 0, err
		}
		added++
	}
	return []byte(js), added, nil
}

// RemoveHooks tira as entradas do Argus; eventos que ficam vazios somem.
func RemoveHooks(data []byte, hook string) ([]byte, int, error) {
	js := string(data)
	removed := 0
	for _, e := range ClaudeEvents {
		key := "hooks." + e.Event
		arr := gjson.Get(js, key)
		if !arr.IsArray() {
			continue
		}
		items := arr.Array()
		for i := len(items) - 1; i >= 0; i-- {
			if strings.Contains(items[i].Raw, hook) {
				var err error
				js, err = sjson.Delete(js, fmt.Sprintf("%s.%d", key, i))
				if err != nil {
					return nil, removed, err
				}
				removed++
			}
		}
		if len(gjson.Get(js, key).Array()) == 0 {
			js, _ = sjson.Delete(js, key)
		}
	}
	return []byte(js), removed, nil
}

func addHooksFile(path, hook string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	js, added, err := AddHooks(data, hook)
	if err != nil {
		return "", fmt.Errorf("%s: %w; nada foi alterado", path, err)
	}
	if added == 0 {
		return "(já instalados)", nil
	}
	if err := writeWithBackup(path, data, js); err != nil {
		return "", err
	}
	return fmt.Sprintf("(+%d eventos; backup ao lado)", added), nil
}

// writeWithBackup grava no arquivo real (settings.json pode ser link), mantém a
// permissão e deixa uma cópia do original ao lado.
func writeWithBackup(path string, old, js []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if len(strings.TrimSpace(string(old))) > 2 {
		bak := fmt.Sprintf("%s.argus-bak-%d", path, time.Now().Unix())
		if err := os.WriteFile(bak, old, 0o600); err != nil {
			return err
		}
	}
	out := pretty.PrettyOptions(js, &pretty.Options{Indent: "  ", Width: 120})
	tmp := path + ".argus-tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return err
	}
	_ = os.Chmod(tmp, mode)
	return os.Rename(tmp, path)
}
