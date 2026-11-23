package procs

/*
#include <libproc.h>
#include <sys/proc_info.h>
#include <mach/mach_time.h>

// task_usage: CPU time in ns (the kernel counts Mach time units) and
// resident memory of one process.
static int task_usage(int pid, unsigned long long *cpu, unsigned long long *rss) {
	struct proc_taskinfo ti;
	if (proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &ti, sizeof(ti)) != (int)sizeof(ti)) {
		return -1;
	}
	static mach_timebase_info_data_t tb;
	if (tb.denom == 0) {
		mach_timebase_info(&tb);
	}
	*cpu = (ti.pti_total_user + ti.pti_total_system) * tb.numer / tb.denom;
	*rss = ti.pti_resident_size;
	return 0;
}
*/
import "C"

import "golang.org/x/sys/unix"

func usage(pid int) (cpu, rss uint64, ok bool) {
	var c, r C.ulonglong
	if C.task_usage(C.int(pid), &c, &r) != 0 {
		return 0, 0, false
	}
	return uint64(c), uint64(r), true
}

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
