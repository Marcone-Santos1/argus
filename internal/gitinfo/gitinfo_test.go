package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	in := Parse("## main...origin/main [ahead 2, behind 1]\n M a.go\n?? b.go\n")
	if in.Branch != "main" || in.Changed != 2 || in.Ahead != 2 {
		t.Fatalf("%+v", in)
	}
	if in := Parse("## No commits yet on dev\n"); in.Branch != "dev" || in.Changed != 0 {
		t.Fatalf("new repo: %+v", in)
	}
	if in := Parse("## HEAD (no branch)\n"); in.Branch != "HEAD" {
		t.Fatalf("detached: %+v", in)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Read(dir); ok {
		t.Fatal("not a repository")
	}
	if err := exec.Command("git", "-C", dir, "init", "-q", "-b", "trunk").Run(); err != nil {
		t.Skip("git unavailable")
	}
	_ = os.WriteFile(filepath.Join(dir, "x"), []byte("1"), 0o644)
	in, ok := Read(dir)
	if !ok || in.Branch != "trunk" || in.Changed != 1 {
		t.Fatalf("%+v %v", in, ok)
	}
}
