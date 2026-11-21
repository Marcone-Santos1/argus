package history

import (
	"os"
	"path/filepath"
	"testing"
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
