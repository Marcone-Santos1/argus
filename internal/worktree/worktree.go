// Package worktree creates and removes git worktrees, so parallel sessions in
// the same repository do not step on each other.
package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Marcone-Santos1/argus/internal/i18n"
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

// RepoRoot returns the root of the main repository containing dir (even when
// dir is already a worktree).
func RepoRoot(dir string) (string, error) {
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", errors.New(i18n.T("wt.notRepo"))
	}
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), nil
	}
	return git(dir, "rev-parse", "--show-toplevel")
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Path is where a branch's worktree lives: next to the repository, in
// <repo>.worktrees/<branch>, so it never shows up inside the project.
func Path(repo, branch string) string {
	name := strings.Trim(unsafeChars.ReplaceAllString(branch, "-"), "-.")
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+".worktrees", name)
}

// ValidBranch checks the name with git itself.
func ValidBranch(repo, branch string) bool {
	if strings.TrimSpace(branch) == "" || strings.HasPrefix(branch, "-") {
		return false
	}
	_, err := git(repo, "check-ref-format", "--branch", branch)
	return err == nil
}

// Create creates the worktree; the branch is created from HEAD when it does not exist.
func Create(repo, branch string) (string, error) {
	if !ValidBranch(repo, branch) {
		return "", errors.New(i18n.T("wt.badBranch", branch))
	}
	path := Path(repo, branch)
	if _, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = git(repo, "worktree", "add", path, branch)
		return path, err
	}
	_, err := git(repo, "worktree", "add", "-b", branch, path)
	return path, err
}

// Dirty reports whether the worktree has uncommitted changes (new files included).
func Dirty(path string) (bool, error) {
	out, err := git(path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// Remove deletes a clean worktree. The branch stays.
func Remove(repo, path string) error {
	dirty, err := Dirty(path)
	if err != nil {
		return err
	}
	if dirty {
		return errors.New(i18n.T("wt.dirty"))
	}
	_, err = git(repo, "worktree", "remove", path)
	return err
}
