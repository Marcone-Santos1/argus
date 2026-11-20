package install

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/paths"
)

const hook = "/h/.local/share/argus/hook.sh"

const existing = `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "rtk hook claude"}]}],
    "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "outro-hook stop"}]}]
  }
}`

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, b)
	}
	return m
}

func TestAddHooksKeepsOthers(t *testing.T) {
	out, added, err := AddHooks([]byte(existing), hook)
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ClaudeEvents) {
		t.Fatalf("eventos adicionados: %d", added)
	}
	m := decode(t, out)
	if m["model"] != "opus" {
		t.Error("outras chaves devem ficar intactas")
	}
	hooks := m["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	if len(stop) != 2 || !strings.Contains(string(mustJSON(stop[0])), "outro-hook") {
		t.Fatalf("o hook que já existia continua primeiro: %s", mustJSON(stop))
	}
	if !strings.Contains(string(mustJSON(stop[1])), hook+" stop") {
		t.Fatalf("o do Argus vem depois: %s", mustJSON(stop))
	}
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("eventos que o Argus não usa continuam")
	}

	again, n, err := AddHooks(out, hook)
	if err != nil || n != 0 || !bytes.Equal(again, out) {
		t.Fatalf("instalar de novo não muda nada: n=%d err=%v", n, err)
	}

	back, removed, err := RemoveHooks(out, hook)
	if err != nil || removed != len(ClaudeEvents) {
		t.Fatalf("remoção: %d %v", removed, err)
	}
	mb := decode(t, back)
	hb := mb["hooks"].(map[string]any)
	if len(hb) != 2 || len(hb["Stop"].([]any)) != 1 {
		t.Fatalf("depois de remover, sobra só o que existia: %s", back)
	}
}

func TestAddHooksEdgeCases(t *testing.T) {
	out, added, err := AddHooks(nil, hook)
	if err != nil || added != len(ClaudeEvents) {
		t.Fatalf("arquivo vazio: %d %v", added, err)
	}
	decode(t, out)
	if _, _, err := AddHooks([]byte(`{"hooks": [`), hook); err == nil {
		t.Fatal("JSON inválido não pode ser reescrito")
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestInstallEndToEnd(t *testing.T) {
	home := t.TempDir()
	p := paths.For(home, "")
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
	// A conta da empresa aponta para o mesmo arquivo (link).
	if err := os.Symlink(settings, filepath.Join(work, "settings.json")); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := Install(p, &log); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(filepath.Join(work, "settings.json"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("o link do settings.json da outra conta deve continuar link")
	}
	if fi, _ := os.Stat(settings); fi.Mode().Perm() != 0o600 {
		t.Errorf("permissão mantida: %v", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(settings)
	if strings.Count(string(data), p.Hook()+" stop") != 1 {
		t.Fatalf("o arquivo compartilhado recebe o hook uma vez só:\n%s", data)
	}
	baks, _ := filepath.Glob(settings + ".argus-bak-*")
	if len(baks) != 1 {
		t.Errorf("backup ao lado: %v", baks)
	}
	plugin, err := os.ReadFile(filepath.Join(p.OpencodePlugins(), PluginFile))
	if err != nil || !strings.Contains(string(plugin), p.Opencode()) || strings.Contains(string(plugin), "__OPENCODE_DIR__") {
		t.Fatalf("plugin do opencode com a pasta certa: %v", err)
	}
	conf, _ := os.ReadFile(p.TmuxConf())
	if strings.Contains(string(conf), "__STATE_DIR__") || !strings.Contains(string(conf), p.State) {
		t.Fatal("tmux.conf com a pasta de estado")
	}

	log.Reset()
	if err := Uninstall(p, &log); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(settings)
	if strings.Contains(string(data), p.Hook()) {
		t.Fatalf("uninstall tira os hooks:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(p.OpencodePlugins(), PluginFile)); err == nil {
		t.Fatal("uninstall tira o plugin")
	}
	for _, d := range []string{p.State, p.Share, p.Conf} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("uninstall apaga %s", d)
		}
	}
}

// O hook é shell puro; aqui ele roda de verdade.
func TestHookScript(t *testing.T) {
	if _, err := os.Stat("/bin/dash"); err != nil {
		t.Skip("sem /bin/dash")
	}
	state := t.TempDir()
	script := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(script, []byte(HookScript(state)), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(id, arg string) string {
		cmd := exec.Command(script, arg)
		cmd.Env = []string{"ARGUS_ID=" + id}
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) > 0 {
			t.Fatalf("hook %s: err=%v saída=%q (não pode escrever nada)", arg, err, out)
		}
		b, _ := os.ReadFile(filepath.Join(state, id))
		return strings.TrimSpace(string(b))
	}
	steps := []struct{ arg, want string }{
		{"start", "idle"},
		{"notify", "idle"}, // aviso sem estar trabalhando não muda nada
		{"prompt", "working"},
		{"tool", "working"},
		{"notify", "waiting"},
		{"tool", "working"}, // permissão respondida
		{"perm", "waiting"},
		{"stop", "done"},
		{"desconhecido", "done"},
		{"end", "ended"},
	}
	for _, s := range steps {
		if got := run("abc123", s.arg); got != s.want {
			t.Fatalf("depois de %s: %q, quer %q", s.arg, got, s.want)
		}
	}
	// Fora do Argus (sem ARGUS_ID) ou com id estranho, não faz nada.
	cmd := exec.Command(script, "prompt")
	cmd.Env = []string{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if got := run("../x", "prompt"); got != "" {
		t.Fatalf("id inválido não pode escrever fora da pasta: %q", got)
	}
	entries, _ := os.ReadDir(state)
	if len(entries) != 1 {
		t.Fatalf("só o arquivo da sessão: %v", entries)
	}
}
