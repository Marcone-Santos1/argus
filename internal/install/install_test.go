package install

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
)

const hook = "/h/.local/share/argus/hook.sh"

const existing = `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "rtk hook claude"}]}],
    "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "other-hook stop"}]}]
  }
}`

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b)
	}
	return m
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestAddHooksKeepsOthers(t *testing.T) {
	out, added, err := AddHooks([]byte(existing), hook, ClaudeEvents, true)
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ClaudeEvents) {
		t.Fatalf("events added: %d", added)
	}
	m := decode(t, out)
	if m["model"] != "opus" {
		t.Error("other keys must stay intact")
	}
	hooks := m["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	if len(stop) != 2 || !strings.Contains(mustJSON(stop[0]), "other-hook") {
		t.Fatalf("the existing hook stays first: %s", mustJSON(stop))
	}
	if !strings.Contains(mustJSON(stop[1]), hook+" stop") {
		t.Fatalf("the Argus hook comes after it: %s", mustJSON(stop))
	}
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("events Argus does not use stay")
	}

	again, n, err := AddHooks(out, hook, ClaudeEvents, true)
	if err != nil || n != 0 || !bytes.Equal(again, out) {
		t.Fatalf("installing again changes nothing: n=%d err=%v", n, err)
	}

	back, removed, err := RemoveHooks(out, hook, ClaudeEvents)
	if err != nil || removed != len(ClaudeEvents) {
		t.Fatalf("removal: %d %v", removed, err)
	}
	hb := decode(t, back)["hooks"].(map[string]any)
	if len(hb) != 2 || len(hb["Stop"].([]any)) != 1 {
		t.Fatalf("after removing, only the original hooks remain: %s", back)
	}
}

func TestAddHooksEdgeCases(t *testing.T) {
	out, added, err := AddHooks(nil, hook, GeminiEvents, false)
	if err != nil || added != len(GeminiEvents) {
		t.Fatalf("empty file: %d %v", added, err)
	}
	m := decode(t, out)
	entry := m["hooks"].(map[string]any)["BeforeAgent"].([]any)[0].(map[string]any)
	if _, has := entry["matcher"]; has {
		t.Error("Gemini entries carry no matcher")
	}
	back, _, _ := RemoveHooks(out, hook, GeminiEvents)
	if _, has := decode(t, back)["hooks"]; has {
		t.Errorf("a hooks object Argus created is removed when empty: %s", back)
	}
	if _, _, err := AddHooks([]byte(`{"hooks": [`), hook, ClaudeEvents, true); err == nil {
		t.Fatal("invalid JSON must not be rewritten")
	}
}

func TestTargets(t *testing.T) {
	home := t.TempDir()
	p := paths.For(home, "")
	cfg, err := config.Parse(`
[[profile]]
name = "a"
command = "claude"
[[profile]]
name = "b"
command = "claude"
env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }
[[profile]]
name = "g"
command = "gemini"
`, home)
	if err != nil {
		t.Fatal(err)
	}
	ts := Targets(cfg, p)
	var got []string
	for _, x := range ts {
		got = append(got, x.Tool+" "+x.File)
	}
	want := []string{
		"claude " + filepath.Join(home, ".claude", "settings.json"),
		"claude " + filepath.Join(home, ".claude-work", "settings.json"),
		"gemini " + filepath.Join(home, ".gemini", "settings.json"),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("targets:\n%s", strings.Join(got, "\n"))
	}
}

func TestInstallEndToEnd(t *testing.T) {
	home := t.TempDir()
	p := paths.For(home, "")
	if err := os.MkdirAll(p.Conf, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `
[[profile]]
name = "personal"
command = "claude"
[[profile]]
name = "work"
command = "claude"
env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }
[[profile]]
name = "opencode"
command = "opencode"
`
	if err := os.WriteFile(p.Config(), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(home, ".claude")
	work := filepath.Join(home, ".claude-work")
	for _, d := range []string{claude, work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	settings := filepath.Join(claude, "settings.json")
	if err := os.WriteFile(settings, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	// The work account points at the same file (a symlink).
	if err := os.Symlink(settings, filepath.Join(work, "settings.json")); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := Install(p, &log); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(work, "settings.json"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the other account's settings.json symlink must stay a symlink")
	}
	if fi, _ := os.Stat(settings); fi.Mode().Perm() != 0o600 {
		t.Errorf("permission kept: %v", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(settings)
	if strings.Count(string(data), p.Hook()+" stop") != 1 {
		t.Fatalf("the shared file gets the hook once:\n%s", data)
	}
	if baks, _ := filepath.Glob(settings + ".argus-bak-*"); len(baks) != 1 {
		t.Errorf("backup next to it: %v", baks)
	}
	plugin, err := os.ReadFile(filepath.Join(p.OpencodePlugins(), PluginFile))
	if err != nil || !strings.Contains(string(plugin), p.Opencode()) || strings.Contains(string(plugin), "__OPENCODE_DIR__") {
		t.Fatalf("opencode plugin with the right folder: %v", err)
	}
	conf, _ := os.ReadFile(p.TmuxConf())
	if strings.Contains(string(conf), "__") || !strings.Contains(string(conf), p.State) {
		t.Fatal("tmux.conf with every placeholder filled")
	}

	log.Reset()
	if err := Uninstall(p, &log); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settings)
	if strings.Contains(string(data), p.Hook()) {
		t.Fatalf("uninstall removes the hooks:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(p.OpencodePlugins(), PluginFile)); err == nil {
		t.Fatal("uninstall removes the plugin")
	}
	for _, d := range []string{p.State, p.Share, p.Conf} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("uninstall deletes %s", d)
		}
	}
}

// The hook is plain shell; here it really runs.
func TestHookScript(t *testing.T) {
	state := t.TempDir()
	script := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(script, []byte(HookScript(state, Shell())), 0o755); err != nil {
		t.Fatal(err)
	}
	runIn := func(id, arg, stdin string) (string, string) {
		cmd := exec.Command(script, arg)
		cmd.Env = []string{"ARGUS_ID=" + id}
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) > 0 {
			t.Fatalf("hook %s: err=%v output=%q (it must print nothing)", arg, err, out)
		}
		b, _ := os.ReadFile(filepath.Join(state, id))
		st, sid, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
		return st, sid
	}
	run := func(id, arg string) string {
		st, _ := runIn(id, arg, "")
		return st
	}

	// SessionStart carries the conversation id; it survives every later event.
	if st, sid := runIn("abc123", "start", `{"session_id":"7f3a-c2","cwd":"/p"}`+"\n"); st != "idle" || sid != "7f3a-c2" {
		t.Fatalf("start: %q %q", st, sid)
	}
	if _, sid := runIn("abc123", "prompt", ""); sid != "7f3a-c2" {
		t.Fatalf("the id stays: %q", sid)
	}
	if _, sid := runIn("abc123", "start", `{"session_id": "new-id", "x": 1}`); sid != "new-id" {
		t.Fatalf("a new SessionStart (/clear, /resume) updates the id, spaced JSON too: %q", sid)
	}
	if _, sid := runIn("abc123", "start", `{"session_id":"bad;id"}`); sid != "new-id" {
		t.Fatalf("an unsafe id is ignored and the old one kept: %q", sid)
	}

	steps := []struct{ arg, want string }{
		{"start", "idle"},
		{"notify", "idle"}, // a notification while not working changes nothing
		{"prompt", "working"},
		{"tool", "working"},
		{"notify", "waiting"},
		{"tool", "working"}, // permission answered
		{"perm", "waiting"},
		{"stop", "done"},
		{"unknown", "done"},
		{"end", "ended"},
	}
	for _, s := range steps {
		if got := run("abc123", s.arg); got != s.want {
			t.Fatalf("after %s: %q, want %q", s.arg, got, s.want)
		}
	}
	// Outside Argus (no ARGUS_ID) or with an odd id, it does nothing.
	cmd := exec.Command(script, "prompt")
	cmd.Env = []string{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if got := run("../x", "prompt"); got != "" {
		t.Fatalf("an invalid id cannot write outside the folder: %q", got)
	}
	if entries, _ := os.ReadDir(state); len(entries) != 1 {
		t.Fatalf("only the session's file: %v", entries)
	}
}
