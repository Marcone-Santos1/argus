package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
)

func TestOrgKeys(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"{}":       "",
		"not json": "",
		`{"env":{"OTEL_LOG_USER_PROMPTS":"1","CLAUDE_CODE_ENABLE_TELEMETRY":"1"},"permissions":{}}`: "env.CLAUDE_CODE_ENABLE_TELEMETRY,env.OTEL_LOG_USER_PROMPTS,permissions",
	}
	for in, want := range cases {
		if got := strings.Join(orgKeys([]byte(in)), ","); got != want {
			t.Errorf("orgKeys(%q) = %q, want %q", in, got, want)
		}
	}
}

// The panel warns once per change of an account's organization settings,
// stays quiet while they are empty, and tells when they are removed.
func TestOrgCheck(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, ".claude-work")
	_ = os.MkdirAll(work, 0o755)
	cfg, _ := config.Parse("[[profile]]\nname = \"pes\"\ncommand = \"claude\"\n[[profile]]\nname = \"emp\"\ncommand = \"claude\"\nenv = { CLAUDE_CONFIG_DIR = \"~/.claude-work\" }\n", home)
	p := paths.For(home, "org-test")
	_ = os.MkdirAll(p.State, 0o755)
	var got []string
	pn := New(Deps{Paths: p, Config: cfg, Notify: func(_, body string) { got = append(got, body) }})
	file := filepath.Join(work, "remote-settings.json")
	now := time.Now()
	step := func(content string) {
		t.Helper()
		if content == "-" {
			_ = os.Remove(file)
		} else {
			_ = os.WriteFile(file, []byte(content), 0o644)
		}
		now = now.Add(2 * orgCheckEvery)
		pn.orgCheck(now)
	}

	step("{}")
	if len(got) != 0 {
		t.Fatalf("empty settings are quiet: %v", got)
	}
	step(`{"env":{"OTEL_LOG_USER_PROMPTS":"1"}}`)
	if len(got) != 1 || !strings.Contains(got[0], "emp") || !strings.Contains(got[0], "env.OTEL_LOG_USER_PROMPTS") {
		t.Fatalf("a change names the account and what was set: %v", got)
	}
	step(`{"env":{"OTEL_LOG_USER_PROMPTS":"1"}}`)
	if len(got) != 1 {
		t.Fatalf("no new warning without a change: %v", got)
	}
	// A restarted panel remembers what it already told.
	pn2 := New(Deps{Paths: p, Config: cfg, Notify: func(_, body string) { got = append(got, body) }})
	pn2.orgCheck(now)
	if len(got) != 1 {
		t.Fatalf("a restart does not repeat the warning: %v", got)
	}
	step("-")
	if len(got) != 2 {
		t.Fatalf("removing the settings is told too: %v", got)
	}
	step("{}")
	if len(got) != 2 {
		t.Fatalf("no file and {} are the same: %v", got)
	}
}
