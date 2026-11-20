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
		t.Skip("git não instalado")
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
	if got := Path("/p/api", "feat/login novo"); got != "/p/api.worktrees/feat-login-novo" {
		t.Fatalf("Path: %s", got)
	}
}

func TestLifecycle(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "sub")
	_ = os.MkdirAll(sub, 0o755)
	if root, err := RepoRoot(sub); err != nil || root != repo {
		t.Fatalf("RepoRoot de subpasta: %q %v", root, err)
	}
	if _, err := RepoRoot(t.TempDir()); err == nil {
		t.Fatal("pasta fora de repositório deveria dar erro")
	}
	if ValidBranch(repo, "") || ValidBranch(repo, "-x") || ValidBranch(repo, "a..b") || !ValidBranch(repo, "feat/x") {
		t.Fatal("validação de branch")
	}

	path, err := Create(repo, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if root, err := RepoRoot(path); err != nil || root != repo {
		t.Fatalf("RepoRoot de dentro da worktree aponta o repo principal: %q %v", root, err)
	}
	if dirty, err := Dirty(path); err != nil || dirty {
		t.Fatalf("worktree nova está limpa: %v %v", dirty, err)
	}

	if err := os.WriteFile(filepath.Join(path, "novo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(repo, path); err == nil {
		t.Fatal("worktree com arquivo novo não pode ser apagada")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("a worktree suja continua lá")
	}
	_ = os.Remove(filepath.Join(path, "novo.txt"))
	if err := Remove(repo, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("worktree limpa foi apagada")
	}

	// O branch ficou; criar de novo reaproveita.
	if _, err := Create(repo, "feat/x"); err != nil {
		t.Fatalf("recriar com branch existente: %v", err)
	}
}
