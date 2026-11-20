package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "api")
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return repo
}

func TestPath(t *testing.T) {
	if got := Path("/p/api", "feat/new login"); got != "/p/api.worktrees/feat-new-login" {
		t.Fatalf("Path: %s", got)
	}
}

func TestLifecycle(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "sub")
	_ = os.MkdirAll(sub, 0o755)
	if root, err := RepoRoot(sub); err != nil || root != repo {
		t.Fatalf("RepoRoot of a subfolder: %q %v", root, err)
	}
	if _, err := RepoRoot(t.TempDir()); err == nil {
		t.Fatal("a folder outside a repository must fail")
	}
	if ValidBranch(repo, "") || ValidBranch(repo, "-x") || ValidBranch(repo, "a..b") || !ValidBranch(repo, "feat/x") {
		t.Fatal("branch validation")
	}

	path, err := Create(repo, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if root, err := RepoRoot(path); err != nil || root != repo {
		t.Fatalf("RepoRoot inside the worktree points to the main repo: %q %v", root, err)
	}
	if dirty, err := Dirty(path); err != nil || dirty {
		t.Fatalf("a new worktree is clean: %v %v", dirty, err)
	}

	if err := os.WriteFile(filepath.Join(path, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(repo, path); err == nil {
		t.Fatal("a worktree with a new file cannot be deleted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the dirty worktree stays")
	}
	_ = os.Remove(filepath.Join(path, "new.txt"))
	if err := Remove(repo, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the clean worktree was deleted")
	}

	// The branch stays; creating again reuses it.
	if _, err := Create(repo, "feat/x"); err != nil {
		t.Fatalf("recreate with an existing branch: %v", err)
	}
}
