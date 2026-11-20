// Package worktree cria e remove worktrees do git para sessões paralelas no
// mesmo repositório.
package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// RepoRoot devolve a raiz do repositório principal que contém dir (mesmo se
// dir já for uma worktree).
func RepoRoot(dir string) (string, error) {
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", errors.New("a pasta não é um repositório git")
	}
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), nil
	}
	return git(dir, "rev-parse", "--show-toplevel")
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Path é onde a worktree de um branch fica: ao lado do repositório, em
// <repo>.worktrees/<branch>, para não aparecer dentro do próprio projeto.
func Path(repo, branch string) string {
	name := strings.Trim(unsafeChars.ReplaceAllString(branch, "-"), "-.")
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+".worktrees", name)
}

// ValidBranch confere o nome com o próprio git.
func ValidBranch(repo, branch string) bool {
	if strings.TrimSpace(branch) == "" || strings.HasPrefix(branch, "-") {
		return false
	}
	_, err := git(repo, "check-ref-format", "--branch", branch)
	return err == nil
}

// Create cria a worktree; o branch é criado a partir do HEAD se ainda não existir.
func Create(repo, branch string) (string, error) {
	if !ValidBranch(repo, branch) {
		return "", fmt.Errorf("nome de branch inválido: %q", branch)
	}
	path := Path(repo, branch)
	if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = git(repo, "worktree", "add", path, branch)
		return path, err
	}
	_, err := git(repo, "worktree", "add", "-b", branch, path)
	return path, err
}

// Dirty diz se a worktree tem mudanças sem commit (inclusive arquivos novos).
func Dirty(path string) (bool, error) {
	out, err := git(path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// Remove apaga uma worktree limpa. O branch continua existindo.
func Remove(repo, path string) error {
	dirty, err := Dirty(path)
	if err != nil {
		return err
	}
	if dirty {
		return errors.New("a worktree tem mudanças sem commit; ela foi mantida")
	}
	_, err = git(repo, "worktree", "remove", path)
	return err
}
