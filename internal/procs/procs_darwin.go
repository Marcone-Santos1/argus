package procs

import "golang.org/x/sys/unix"

// list reads the process table with one sysctl (no child process).
func list() ([]Proc, error) {
	ks, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	out := make([]Proc, 0, len(ks))
	for _, k := range ks {
		c := k.Proc.P_comm[:]
		n := 0
		for n < len(c) && c[n] != 0 {
			n++
		}
		b := make([]byte, n)
		for i := 0; i < n; i++ {
			b[i] = byte(c[i])
		}
		out = append(out, Proc{PID: int(k.Proc.P_pid), PPID: int(k.Eproc.Ppid), Comm: string(b)})
	}
	return out, nil
}
