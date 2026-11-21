// Package gitinfo reads a folder's branch and how many files changed, with
// one git process. The panel calls it only when a session's turn ends.
package gitinfo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Info is a repository's state.
type Info struct {
	Branch  string
	Changed int // files modified, added, deleted or untracked
	Ahead   int // commits not pushed
}

// Read runs `git status` in dir; ok is false outside a repository.
func Read(dir string) (Info, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "-C", dir, "status", "--porcelain=v1", "-b", "--untracked-files=normal")
	out, err := cmd.Output()
	if err != nil {
		return Info{}, false
	}
	return Parse(string(out)), true
}

// Parse interprets `git status --porcelain=v1 -b`.
func Parse(out string) Info {
	var in Info
	for i, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if i == 0 && strings.HasPrefix(l, "## ") {
			h := strings.TrimPrefix(l, "## ")
			if b, ok := strings.CutPrefix(h, "No commits yet on "); ok {
				h = b
			}
			name, rest, _ := strings.Cut(h, "...")
			name, _, _ = strings.Cut(name, " ")
			in.Branch = name
			if j := strings.Index(rest, "ahead "); j >= 0 {
				n := 0
				for _, c := range rest[j+6:] {
					if c < '0' || c > '9' {
						break
					}
					n = n*10 + int(c-'0')
				}
				in.Ahead = n
			}
			continue
		}
		if strings.TrimSpace(l) != "" {
			in.Changed++
		}
	}
	return in
}
