package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func line(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b) + "\n"
}

func writeTranscript(t *testing.T, dir, id string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTranscriptSmall(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s1",
		line(map[string]any{"type": "user", "cwd": "/p/api", "isMeta": true, "message": map[string]any{"content": "interno"}}),
		line(map[string]any{"type": "user", "cwd": "/p/api", "message": map[string]any{"content": "<command>x</command>"}}),
		line(map[string]any{"type": "user", "cwd": "/p/api", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "  corrige   o teste  "}}}}),
		line(map[string]any{"type": "session-owner", "accountUuid": "acc-1"}),
	)
	fi, _ := os.Stat(path)
	e, ok := ParseTranscript(path, fi.Size())
	if !ok {
		t.Fatal("transcrição válida rejeitada")
	}
	if e.ID != "s1" || e.Dir != "/p/api" || e.Account != "acc-1" || e.Tool != Claude {
		t.Fatalf("entrada: %+v", e)
	}
	if e.Title != "corrige o teste" {
		t.Fatalf("sem título, usa o primeiro prompt de verdade: %q", e.Title)
	}
}

func TestParseTranscriptLatestTitleWins(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s2",
		line(map[string]any{"type": "user", "cwd": "/p/x"}),
		line(map[string]any{"type": "ai-title", "aiTitle": "Título velho"}),
		line(map[string]any{"type": "custom-title", "customTitle": "Título que o usuário deu"}),
	)
	fi, _ := os.Stat(path)
	e, _ := ParseTranscript(path, fi.Size())
	if e.Title != "Título que o usuário deu" {
		t.Fatalf("título: %q", e.Title)
	}
}

// Uma transcrição grande: a pasta está só no começo e o título só no fim, com
// uma linha gigante no meio. O parser lê só as pontas.
func TestParseTranscriptLarge(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 2*Chunk)
	path := writeTranscript(t, dir, "s3",
		line(map[string]any{"type": "user", "cwd": `/p/com "aspas"`}),
		line(map[string]any{"type": "session-owner", "accountUuid": "acc-head"}),
		line(map[string]any{"type": "api-request-blob", "blob": big}),
		line(map[string]any{"type": "ai-title", "aiTitle": "No fim"}),
	)
	fi, _ := os.Stat(path)
	e, ok := ParseTranscript(path, fi.Size())
	if !ok || e.Dir != `/p/com "aspas"` || e.Title != "No fim" || e.Account != "acc-head" {
		t.Fatalf("entrada: ok=%v %+v", ok, e)
	}
}

func TestParseTranscriptWithoutCwd(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s4", line(map[string]any{"type": "ai-title", "aiTitle": "x"}))
	fi, _ := os.Stat(path)
	if _, ok := ParseTranscript(path, fi.Size()); ok {
		t.Fatal("sem pasta não dá para retomar")
	}
}

func TestLoader(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, "projects")
	alive := filepath.Join(home, "api")
	if err := os.MkdirAll(filepath.Join(projects, "-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(alive, 0o755); err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(projects, "-api")
	old := writeTranscript(t, d, "old",
		line(map[string]any{"type": "user", "cwd": alive}),
		line(map[string]any{"type": "session-owner", "accountUuid": "acc-work"}),
		line(map[string]any{"type": "ai-title", "aiTitle": "Antiga"}))
	newer := writeTranscript(t, d, "new",
		line(map[string]any{"type": "user", "cwd": alive}),
		line(map[string]any{"type": "ai-title", "aiTitle": "Nova"}))
	writeTranscript(t, d, "gone", line(map[string]any{"type": "user", "cwd": filepath.Join(home, "apagada")}))
	_ = os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	_ = os.Chtimes(newer, time.Now(), time.Now())

	l := &Loader{ClaudeProjects: projects, Cache: filepath.Join(home, "cache.json"),
		Accounts: map[string]string{"acc-work": "claude-work"}}
	es := l.Load()
	if len(es) != 2 {
		t.Fatalf("pasta apagada some; esperava 2: %+v", es)
	}
	if es[0].Title != "Nova" || es[1].Title != "Antiga" {
		t.Fatalf("mais recente primeiro: %s, %s", es[0].Title, es[1].Title)
	}
	if es[1].Profile != "claude-work" {
		t.Errorf("conta registrada vira perfil: %+v", es[1])
	}
	if es[0].Profile != "claude-work" {
		t.Errorf("sem conta e com uma conta só configurada, usa ela: %+v", es[0])
	}
	if _, err := os.Stat(l.Cache); err != nil {
		t.Fatal("o índice deveria ser gravado")
	}
	// Segunda leitura usa o índice e dá o mesmo resultado.
	if again := l.Load(); len(again) != 2 || again[0].ID != es[0].ID {
		t.Fatalf("com índice: %+v", again)
	}
}

func TestOpencodeRows(t *testing.T) {
	out := "ses_a\x1f/p/api\x1fArrumar build\x1f1795000000000\nquebrada\n"
	es := ParseOpencodeRows(out, "opencode")
	if len(es) != 1 {
		t.Fatalf("linhas: %+v", es)
	}
	e := es[0]
	if e.Tool != Opencode || e.ID != "ses_a" || e.Dir != "/p/api" || e.Title != "Arrumar build" ||
		e.Profile != "opencode" || e.Updated.UnixMilli() != 1795000000000 {
		t.Fatalf("entrada: %+v", e)
	}
}

func TestResumeArgsAndSafeID(t *testing.T) {
	if got := ResumeArgs(Opencode, "ses_1"); strings.Join(got, " ") != "--session ses_1" {
		t.Errorf("opencode: %v", got)
	}
	if got := ResumeArgs(Claude, "abc"); strings.Join(got, " ") != "--resume abc" {
		t.Errorf("claude: %v", got)
	}
	for id, want := range map[string]bool{"f297298e-030f": true, "ses_e252cb55dffe": true, "a;rm -rf": false, "": false, "$(x)": false} {
		if SafeID(id) != want {
			t.Errorf("SafeID(%q) != %v", id, want)
		}
	}
}

func TestAccountOf(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if AccountOf(path) != "" {
		t.Fatal("arquivo ausente")
	}
	_ = os.WriteFile(path, []byte(`{"oauthAccount":{"accountUuid":"u-1"},"outro":1}`), 0o600)
	if got := AccountOf(path); got != "u-1" {
		t.Fatalf("conta: %q", got)
	}
}
