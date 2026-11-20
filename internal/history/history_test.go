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
		line(map[string]any{"type": "user", "cwd": "/p/api", "isMeta": true, "message": map[string]any{"content": "internal"}}),
		line(map[string]any{"type": "user", "cwd": "/p/api", "message": map[string]any{"content": "<command>x</command>"}}),
		line(map[string]any{"type": "user", "cwd": "/p/api", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "  fix   the test  "}}}}),
		line(map[string]any{"type": "session-owner", "accountUuid": "acc-1"}),
	)
	fi, _ := os.Stat(path)
	e, ok := ParseTranscript(path, fi.Size())
	if !ok {
		t.Fatal("valid transcript rejected")
	}
	if e.ID != "s1" || e.Dir != "/p/api" || e.Account != "acc-1" || e.Tool != Claude {
		t.Fatalf("entry: %+v", e)
	}
	if e.Title != "fix the test" {
		t.Fatalf("without a title, the first real prompt is used: %q", e.Title)
	}
}

func TestParseTranscriptLatestTitleWins(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s2",
		line(map[string]any{"type": "user", "cwd": "/p/x"}),
		line(map[string]any{"type": "ai-title", "aiTitle": "Old title"}),
		line(map[string]any{"type": "custom-title", "customTitle": "Title the user gave"}),
	)
	fi, _ := os.Stat(path)
	e, _ := ParseTranscript(path, fi.Size())
	if e.Title != "Title the user gave" {
		t.Fatalf("title: %q", e.Title)
	}
}

// A large transcript: the folder is only at the start and the title only at
// the end, with a huge line in between. The parser reads only the two ends.
func TestParseTranscriptLarge(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 2*Chunk)
	path := writeTranscript(t, dir, "s3",
		line(map[string]any{"type": "user", "cwd": `/p/with "quotes"`}),
		line(map[string]any{"type": "session-owner", "accountUuid": "acc-head"}),
		line(map[string]any{"type": "api-request-blob", "blob": big}),
		line(map[string]any{"type": "ai-title", "aiTitle": "At the end"}),
	)
	fi, _ := os.Stat(path)
	e, ok := ParseTranscript(path, fi.Size())
	if !ok || e.Dir != `/p/with "quotes"` || e.Title != "At the end" || e.Account != "acc-head" {
		t.Fatalf("entry: ok=%v %+v", ok, e)
	}
}

func TestParseTranscriptWithoutCwd(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s4", line(map[string]any{"type": "ai-title", "aiTitle": "x"}))
	fi, _ := os.Stat(path)
	if _, ok := ParseTranscript(path, fi.Size()); ok {
		t.Fatal("without a folder it cannot be resumed")
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
		line(map[string]any{"type": "ai-title", "aiTitle": "Old"}))
	newer := writeTranscript(t, d, "new",
		line(map[string]any{"type": "user", "cwd": alive}),
		line(map[string]any{"type": "ai-title", "aiTitle": "New"}))
	writeTranscript(t, d, "gone", line(map[string]any{"type": "user", "cwd": filepath.Join(home, "deleted")}))
	_ = os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	_ = os.Chtimes(newer, time.Now(), time.Now())

	l := &Loader{ClaudeProjects: projects, Cache: filepath.Join(home, "cache.json"),
		Accounts: map[string]string{"acc-work": "claude-work"}}
	es := l.Load()
	if len(es) != 2 {
		t.Fatalf("a deleted folder disappears; expected 2: %+v", es)
	}
	if es[0].Title != "New" || es[1].Title != "Old" {
		t.Fatalf("newest first: %s, %s", es[0].Title, es[1].Title)
	}
	if es[1].Profile != "claude-work" {
		t.Errorf("a recorded account maps to a profile: %+v", es[1])
	}
	if es[0].Profile != "claude-work" {
		t.Errorf("no account and a single configured account: use it: %+v", es[0])
	}
	if _, err := os.Stat(l.Cache); err != nil {
		t.Fatal("the index should be written")
	}
	// A second read uses the index and gives the same result.
	if again := l.Load(); len(again) != 2 || again[0].ID != es[0].ID {
		t.Fatalf("with the index: %+v", again)
	}
}

func TestOpencodeRows(t *testing.T) {
	out := "ses_a\x1f/p/api\x1fFix build\x1f1795000000000\nbroken\n"
	es := ParseOpencodeRows(out, "opencode")
	if len(es) != 1 {
		t.Fatalf("rows: %+v", es)
	}
	e := es[0]
	if e.Tool != Opencode || e.ID != "ses_a" || e.Dir != "/p/api" || e.Title != "Fix build" ||
		e.Profile != "opencode" || e.Updated.UnixMilli() != 1795000000000 {
		t.Fatalf("entry: %+v", e)
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
		t.Fatal("missing file")
	}
	_ = os.WriteFile(path, []byte(`{"oauthAccount":{"accountUuid":"u-1"},"outro":1}`), 0o600)
	if got := AccountOf(path); got != "u-1" {
		t.Fatalf("account: %q", got)
	}
}
