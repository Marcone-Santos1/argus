package history

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "-p-api")
	_ = os.MkdirAll(proj, 0o755)
	_ = os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(`{"type":"user","message":{"content":"configure o OAuth da LinkedIn\nagora"}}`+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(proj, "s2.jsonl"), []byte(`{"type":"user","message":{"content":"outra coisa"}}`+"\n"), 0o644)
	// A match split across two read chunks is still found.
	big := `{"type":"user","message":{"content":"` + strings.Repeat("x", searchChunk-40) + "LinkedIn\"}}\n"
	_ = os.WriteFile(filepath.Join(proj, "s3.jsonl"), []byte(big), 0o644)
	l := &Loader{ClaudeProjects: dir}
	hits := l.Search(context.Background(), "linkedin")
	got := map[string]string{}
	for _, h := range hits {
		got[h.Key] = h.Snippet
	}
	if len(got) != 2 || !strings.Contains(got["claude:s1"], "OAuth da LinkedIn agora") || got["claude:s3"] == "" {
		t.Fatalf("hits: %q", got)
	}
	if l.Search(context.Background(), "x") != nil {
		t.Fatal("one letter is too short to search")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if l.Search(ctx, "linkedin") != nil {
		t.Fatal("a cancelled search returns nothing")
	}
}

func TestSearchSkipsNonMessages(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "-p")
	_ = os.MkdirAll(proj, 0o755)
	body := `{"type":"attachment","attachment":{"content":"social media for LinkedIn"},"cwd":"/p/linkedin"}
{"type":"user","isMeta":true,"message":{"content":"LinkedIn reminder"},"cwd":"/p/linkedin"}
{"type":"user","message":{"content":[{"type":"tool_result","content":"LinkedIn"}]},"cwd":"/p/linkedin"}
`
	_ = os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(body), 0o644)
	if hits := (&Loader{ClaudeProjects: dir}).Search(context.Background(), "linkedin"); len(hits) != 0 {
		t.Fatalf("only prompts and replies count: %+v", hits)
	}
}
