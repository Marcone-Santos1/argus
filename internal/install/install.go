// Package install adds and removes what Argus needs outside its own binary:
// the status hook for Claude Code, Codex and Gemini CLI, the opencode plugin
// and the tmux configuration.
package install

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/paths"
)

//go:embed assets/hook.sh
var hookScript string

//go:embed assets/statusline.sh
var statusLineScript string

//go:embed assets/tmux.conf
var tmuxConf string

//go:embed assets/argus-status.ts
var opencodePlugin string

// Argus's themes for Claude Code, picked with /theme ("Argus dark",
// "Argus light"); the panel's look follows the one chosen.
var (
	//go:embed assets/claude-theme-argus-dark.json
	claudeThemeDark string
	//go:embed assets/claude-theme-argus-light.json
	claudeThemeLight string
	//go:embed assets/claude-theme-argus-night.json
	claudeThemeNight string
)

// Argus's themes for the Fresh editor of the code column, the same colors.
var (
	//go:embed assets/fresh-theme-argus-dark.json
	freshThemeDark string
	//go:embed assets/fresh-theme-argus-light.json
	freshThemeLight string
	//go:embed assets/fresh-theme-argus-night.json
	freshThemeNight string
)

// FreshThemes are Fresh's theme files, by name (in ~/.config/fresh/themes).
var FreshThemes = map[string]*string{"argus-dark": &freshThemeDark, "argus-light": &freshThemeLight, "argus-night": &freshThemeNight}

// The Argus theme's sets (key T cycles them): dark, night (purple and pink
// on a blue-gray background) and light.
const (
	ThemeDark  = "dark"
	ThemeNight = "night"
	ThemeLight = "light"
)

// ClaudeThemeSlug is Argus's theme in Claude Code: one theme, "Argus", whose
// file the panel rewrites dark or light (key T). Claude reloads a theme file
// when it changes, so every open session follows without restarting.
const ClaudeThemeSlug = "argus"

// ClaudeTheme is the theme file's content for a set.
func ClaudeTheme(mode string) string {
	switch mode {
	case ThemeLight:
		return claudeThemeLight
	case ThemeNight:
		return claudeThemeNight
	}
	return claudeThemeDark
}

// oldClaudeThemes were two fixed themes, replaced by the one above.
var oldClaudeThemes = []string{"argus-dark", "argus-light"}

// PluginFile is the plugin's name inside opencode's plugin directory.
const PluginFile = "argus-status.ts"

// Event links an agent hook event to the argument the Argus hook receives.
type Event struct {
	Name, Arg string
	Tools     string // Claude's matcher: only these tools ("" = all)
}

// Hook events per agent. The hook script maps the argument to a state.
var (
	ClaudeEvents = []Event{
		{"SessionStart", "start", ""}, {"UserPromptSubmit", "prompt", ""}, {"PermissionRequest", "perm", ""},
		{"Notification", "notify", ""}, {"PostToolUse", "tool", ""}, {"Stop", "stop", ""}, {"SessionEnd", "end", ""},
		{"SubagentStart", "substart", ""}, {"SubagentStop", "substop", ""},
		// A file change: the code column opens it (see the panel's _edit).
		{"PostToolUse", "edit", "Edit|Write|MultiEdit|NotebookEdit"},
	}
	CodexEvents = []Event{
		{"SessionStart", "start", ""}, {"UserPromptSubmit", "prompt", ""}, {"PostToolUse", "tool", ""},
		{"Stop", "stop", ""}, {"SessionEnd", "end", ""},
	}
	GeminiEvents = []Event{
		{"SessionStart", "start", ""}, {"BeforeAgent", "prompt", ""}, {"Notification", "notify", ""},
		{"AfterTool", "tool", ""}, {"AfterAgent", "stop", ""}, {"SessionEnd", "end", ""},
	}
)

// Target is a settings file that receives the hook.
type Target struct {
	Tool    string
	File    string
	Events  []Event
	Matcher bool // whether entries carry "matcher": "" (Claude, Codex)
}

// Targets lists the files to hook, from the configured profiles.
func Targets(cfg *config.Config, p paths.Paths) []Target {
	var ts []Target
	seen := map[string]bool{}
	add := func(t Target) {
		real := t.File
		if r, err := filepath.EvalSymlinks(t.File); err == nil {
			real = r
		}
		if !seen[real] {
			seen[real] = true
			ts = append(ts, t)
		}
	}
	for _, pr := range cfg.ProfilesFor("claude") {
		add(Target{"claude", filepath.Join(pr.ClaudeConfigDir(p.Home), "settings.json"), ClaudeEvents, true})
	}
	if len(cfg.ProfilesFor("codex")) > 0 {
		add(Target{"codex", p.CodexHooks(), CodexEvents, true})
	}
	if len(cfg.ProfilesFor("gemini")) > 0 {
		add(Target{"gemini", p.GeminiSettings(), GeminiEvents, false})
	}
	return ts
}

// Shell is the interpreter for the hook: dash starts in about half the time
// of bash, which is what /bin/sh is on macOS.
func Shell() string {
	if _, err := os.Stat("/bin/dash"); err == nil {
		return "/bin/dash"
	}
	return "/bin/sh"
}

// HookScript is the hook ready for a state directory and interpreter.
func HookScript(stateDir, shell string) string {
	s := strings.ReplaceAll(hookScript, "__STATE_DIR__", stateDir)
	return strings.Replace(s, "#!/bin/dash", "#!"+shell, 1)
}

// StatusLineScript is the status-line script ready for a state directory.
func StatusLineScript(stateDir, shell string) string {
	s := strings.ReplaceAll(statusLineScript, "__STATE_DIR__", stateDir)
	return strings.Replace(s, "#!/bin/dash", "#!"+shell, 1)
}

// AddStatusLine makes Claude Code run the Argus status line, unless the
// file already has a status line of its own (that one is kept).
func AddStatusLine(data []byte, command string) ([]byte, bool, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}
	if !gjson.ValidBytes(data) {
		return nil, false, fmt.Errorf("invalid JSON")
	}
	if gjson.GetBytes(data, "statusLine").Exists() {
		return data, false, nil
	}
	js, err := sjson.SetRawBytes(data, "statusLine", []byte(fmt.Sprintf(`{"type":"command","command":%q}`, command)))
	return js, err == nil, err
}

// RemoveStatusLine removes the status line if it is the Argus one.
func RemoveStatusLine(data []byte, command string) ([]byte, bool) {
	if !strings.Contains(gjson.GetBytes(data, "statusLine.command").String(), command) {
		return data, false
	}
	js, err := sjson.DeleteBytes(data, "statusLine")
	return js, err == nil
}

// TmuxConf is the tmux configuration ready for a state directory.
func TmuxConf(stateDir string) string {
	s := strings.ReplaceAll(tmuxConf, "__STATE_DIR__", stateDir)
	safe := func(k string) string { return strings.ReplaceAll(i18n.T(k), `"`, `'`) }
	s = strings.ReplaceAll(s, "__PLACEHOLDER__", safe("tmux.placeholder"))
	s = strings.ReplaceAll(s, "__COPIED__", safe("tmux.copied"))
	return s + typingBindings()
}

// typingBindings makes typing leave copy mode: after selecting text with the
// mouse, the first key you type exits copy mode and goes to the session, as
// if you had pressed Esc first. Arrows, Page Up/Down, Esc and the mouse keep
// their copy-mode meaning. ("Any" is not bound: it would also catch mouse
// releases and leave copy mode on a plain click.)
func typingBindings() string {
	var b strings.Builder
	b.WriteString("\n# Typing leaves copy mode and goes to the session (generated).\n")
	quote := func(c byte) string {
		if c == '\'' {
			return `"'"`
		}
		return "'" + string(c) + "'"
	}
	for _, table := range []string{"copy-mode", "copy-mode-vi"} {
		for c := byte('!'); c <= '~'; c++ {
			fmt.Fprintf(&b, "bind -T %s %s send -X cancel \\; send-keys -l %s\n", table, quote(c), quote(c))
		}
		for _, k := range []string{"Space", "Enter", "BSpace", "Tab"} {
			fmt.Fprintf(&b, "bind -T %s %s send -X cancel \\; send-keys %s\n", table, k, k)
		}
	}
	return b.String()
}

// OpencodePlugin is the plugin ready for an opencode state directory.
func OpencodePlugin(dir string) string {
	return strings.ReplaceAll(opencodePlugin, "__OPENCODE_DIR__", dir)
}

// Install writes the hook, plugin and tmux config and hooks the agents up.
func Install(p paths.Paths, out io.Writer) error {
	for _, d := range []string{p.State, p.Share, p.Conf, p.Opencode(), p.Usage()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(p.Hook(), []byte(HookScript(p.State, Shell())), 0o755); err != nil {
		return err
	}
	fmt.Fprintln(out, "hook:            ", p.Hook())
	if err := os.WriteFile(p.StatusLine(), []byte(StatusLineScript(p.State, Shell())), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.TmuxConf(), []byte(TmuxConf(p.State)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "tmux.conf:       ", p.TmuxConf())
	if _, err := os.Stat(p.Config()); err != nil {
		if err := os.WriteFile(p.Config(), []byte(config.DefaultTOML), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "config:          ", p.Config(), i18n.T("install.new"))
	} else {
		fmt.Fprintln(out, "config:          ", p.Config(), i18n.T("install.kept"))
	}
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("cli.badConfig"), err)
	}
	for _, t := range Targets(cfg, p) {
		msg, err := addHooksFile(t, p.Hook())
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "hooks %-11s %s %s\n", t.Tool+":", t.File, msg)
		if t.Tool == "claude" {
			fmt.Fprintf(out, "statusLine:        %s %s\n", t.File, addStatusLineFile(t.File, p.StatusLine()))
		}
	}
	// Each Claude account reads themes from its own folder (accounts that
	// share settings.json through a link still have separate folders).
	for _, pr := range cfg.Profiles {
		if pr.Label() != "claude" {
			continue
		}
		dir := filepath.Join(pr.ClaudeConfigDir(p.Home), "themes")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, old := range oldClaudeThemes {
			_ = os.Remove(filepath.Join(dir, old+".json"))
		}
		f := filepath.Join(dir, ClaudeThemeSlug+".json")
		if _, err := os.Stat(f); err != nil { // the panel keeps it dark or light
			if err := os.WriteFile(f, []byte(ClaudeTheme(ThemeDark)), 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "theme claude:     ", f)
	}
	if _, err := exec.LookPath("fresh"); err == nil {
		dir := filepath.Join(p.Home, ".config", "fresh", "themes")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for name, body := range FreshThemes {
			if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(*body), 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "themes fresh:     ", dir, "(argus-dark, argus-night, argus-light)")
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
		fmt.Fprintln(out, "plugin opencode: ", f)
	}
	return nil
}

// Uninstall undoes Install.
func Uninstall(p paths.Paths, out io.Writer) error {
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		cfg, _ = config.Parse(config.DefaultTOML, p.Home)
	}
	for _, t := range Targets(cfg, p) {
		data, err := os.ReadFile(t.File)
		if err != nil {
			continue
		}
		js, n, err := RemoveHooks(data, p.Hook(), t.Events)
		if err != nil {
			return err
		}
		if n > 0 {
			if err := writeWithBackup(t.File, data, js); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "hooks %-11s %s (-%d)\n", t.Tool+":", t.File, n)
		if t.Tool == "claude" {
			if cur, err := os.ReadFile(t.File); err == nil {
				if js, ok := RemoveStatusLine(cur, p.StatusLine()); ok {
					_ = writeWithBackup(t.File, cur, js)
					fmt.Fprintln(out, i18n.T("install.removed"), "statusLine", t.File)
				}
			}
		}
	}
	for _, pr := range cfg.Profiles {
		if pr.Label() == "claude" {
			for _, slug := range append(oldClaudeThemes, ClaudeThemeSlug) {
				_ = os.Remove(filepath.Join(pr.ClaudeConfigDir(p.Home), "themes", slug+".json"))
			}
		}
	}
	plugin := filepath.Join(p.OpencodePlugins(), PluginFile)
	if err := os.Remove(plugin); err == nil {
		fmt.Fprintln(out, i18n.T("install.removed"), plugin)
	}
	for _, d := range []string{p.Share, p.Conf, p.State} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
		fmt.Fprintln(out, i18n.T("install.removed"), d)
	}
	return nil
}

func hookEntry(command string, matcher bool, tools string) string {
	if matcher {
		return fmt.Sprintf(`{"matcher":%q,"hooks":[{"type":"command","command":%q}]}`, tools, command)
	}
	return fmt.Sprintf(`{"hooks":[{"type":"command","command":%q}]}`, command)
}

// AddHooks appends the Argus hook to each event, leaving existing hooks alone.
// It returns the new JSON and how many events got the hook.
func AddHooks(data []byte, hook string, events []Event, matcher bool) ([]byte, int, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}
	if !gjson.ValidBytes(data) {
		return nil, 0, fmt.Errorf("invalid JSON")
	}
	js := string(data)
	added := 0
	for _, e := range events {
		key := "hooks." + e.Name
		cur := gjson.Get(js, key)
		if strings.Contains(cur.Raw, strconvQuote(hook+" "+e.Arg)) {
			continue
		}
		var err error
		if cur.IsArray() {
			js, err = sjson.SetRaw(js, key+".-1", hookEntry(hook+" "+e.Arg, matcher, e.Tools))
		} else {
			js, err = sjson.SetRaw(js, key, "["+hookEntry(hook+" "+e.Arg, matcher, e.Tools)+"]")
		}
		if err != nil {
			return nil, 0, err
		}
		added++
	}
	return []byte(js), added, nil
}

// RemoveHooks removes the Argus entries; events left empty are removed.
func RemoveHooks(data []byte, hook string, events []Event) ([]byte, int, error) {
	js := string(data)
	removed := 0
	for _, e := range events {
		key := "hooks." + e.Name
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
	if gjson.Get(js, "hooks").IsObject() && len(gjson.Get(js, "hooks").Map()) == 0 {
		js, _ = sjson.Delete(js, "hooks")
	}
	return []byte(js), removed, nil
}

func addStatusLineFile(file, command string) string {
	data, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err.Error()
	}
	js, added, err := AddStatusLine(data, command)
	switch {
	case err != nil:
		return i18n.T("install.badJSON", file)
	case !added && strings.Contains(gjson.GetBytes(data, "statusLine.command").String(), command):
		return i18n.T("install.already")
	case !added:
		return i18n.T("install.ownStatusLine")
	}
	if err := writeWithBackup(file, data, js); err != nil {
		return err.Error()
	}
	return i18n.T("install.added", 1)
}

func addHooksFile(t Target, hook string) (string, error) {
	data, err := os.ReadFile(t.File)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	js, added, err := AddHooks(data, hook, t.Events, t.Matcher)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("install.badJSON", t.File))
	}
	if added == 0 {
		return i18n.T("install.already"), nil
	}
	if err := writeWithBackup(t.File, data, js); err != nil {
		return "", err
	}
	return i18n.T("install.added", added), nil
}

// writeWithBackup writes to the real file (settings.json may be a symlink),
// keeps its permissions and leaves a copy of the original next to it.
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

// strconvQuote is the command as it appears inside the JSON (quoted), so
// "hook tool" does not match "hook tools".
func strconvQuote(s string) string { q, _ := json.Marshal(s); return string(q) }
