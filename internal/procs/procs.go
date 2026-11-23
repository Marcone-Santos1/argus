// Package procs finds the shells a session left running in the background
// (Claude Code's run_in_background commands): they outlive the turn, so the
// session reads "done" while tests or builds are still going.
package procs

// Proc is one process: its id, parent and short command name.
type Proc struct {
	PID, PPID int
	Comm      string
}

func isShell(comm string) bool {
	switch comm {
	case "zsh", "bash", "sh", "dash", "fish", "-zsh", "-bash":
		return true
	}
	return false
}

// Shells counts, under root, the shells started by a program that is not a
// shell itself (an agent running a command). Shells inside shells count once,
// and root does not count.
func Shells(all []Proc, root int) int {
	kids := map[int][]Proc{}
	for _, p := range all {
		kids[p.PPID] = append(kids[p.PPID], p)
	}
	n := 0
	var walk func(pid int, parentShell bool, depth int)
	walk = func(pid int, parentShell bool, depth int) {
		if depth > 6 {
			return
		}
		for _, c := range kids[pid] {
			sh := isShell(c.Comm)
			if sh && !parentShell {
				n++
				continue // what runs inside it is that one command
			}
			walk(c.PID, sh, depth+1)
		}
	}
	rootShell := false
	for _, p := range all {
		if p.PID == root {
			rootShell = isShell(p.Comm)
		}
	}
	walk(root, rootShell, 0)
	return n
}

// Table is one read of the process table.
type Table struct {
	all  []Proc
	kids map[int][]int
}

// Read reads the process table once (one sysctl on macOS, no child process).
func Read() (*Table, bool) {
	all, err := list()
	if err != nil {
		return nil, false
	}
	t := &Table{all: all, kids: map[int][]int{}}
	for _, p := range all {
		t.kids[p.PPID] = append(t.kids[p.PPID], p.PID)
	}
	return t, true
}

// Shells is Shells(all, root) on this table.
func (t *Table) Shells(root int) int { return Shells(t.all, root) }

// Tree is root and every process under it.
func (t *Table) Tree(root int) []int {
	out := []int{root}
	for i := 0; i < len(out) && i < 4096; i++ {
		out = append(out, t.kids[out[i]]...)
	}
	return out
}

// Usage is a process's CPU time so far (ns) and resident memory (bytes).
func Usage(pid int) (cpu, rss uint64, ok bool) { return usage(pid) }

// Count returns the background shells under each root, reading the process
// table once. ok is false where it cannot be read.
func Count(roots []int) (map[int]int, bool) {
	all, err := list()
	if err != nil {
		return nil, false
	}
	out := make(map[int]int, len(roots))
	for _, r := range roots {
		out[r] = Shells(all, r)
	}
	return out, true
}
