package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLastReply(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "-p-api")
	_ = os.MkdirAll(proj, 0o755)
	body := `{"type":"user","message":{"content":"fix it"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Old answer"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"\n## **Fixed** the ` + "`login`" + ` test\n\nDetails..."},{"type":"tool_use","name":"Bash"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
`
	_ = os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(body), 0o644)
	if got := LastReply(dir, "s1"); got != "Fixed the login test" {
		t.Fatalf("LastReply: %q", got)
	}
	if LastReply(dir, "missing") != "" || LastReply(dir, "../x") != "" {
		t.Fatal("missing or unsafe id")
	}
}

func TestBackgroundAgents(t *testing.T) {
	projects := t.TempDir()
	conv := filepath.Join(projects, "-p-api")
	sub := filepath.Join(conv, "conv-1", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(conv, "conv-1.jsonl"), []byte("{}\n"), 0o644)
	agent := func(name, meta string, age time.Duration) {
		_ = os.WriteFile(filepath.Join(sub, name+".meta.json"), []byte(meta), 0o644)
		j := filepath.Join(sub, name+".jsonl")
		_ = os.WriteFile(j, []byte("{}\n"), 0o644)
		at := time.Now().Add(-age)
		_ = os.Chtimes(j, at, at)
	}
	agent("agent-a", `{"requestShape":"background"}`, time.Minute)                  // running
	agent("agent-b", `{"requestShape":"background","finished":true}`, time.Minute)  // ended
	agent("agent-c", `{"requestShape":"foreground"}`, time.Minute)                  // part of a turn
	agent("agent-d", `{"requestShape":"background"}`, 2*time.Hour)                  // silent for long: gone
	agent("agent-e", `{"requestShape":"background","finished":false}`, time.Second) // running
	if n := BackgroundAgents(projects, "conv-1", time.Now()); n != 2 {
		t.Fatalf("running background agents: %d, want 2", n)
	}
	if n := BackgroundAgents(projects, "other", time.Now()); n != 0 {
		t.Fatalf("unknown conversation: %d", n)
	}
}
