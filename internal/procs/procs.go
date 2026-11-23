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
