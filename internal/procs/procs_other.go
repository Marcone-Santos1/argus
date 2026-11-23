//go:build !darwin

package procs

import (
	"os"
	"strconv"
	"strings"
)

// list reads /proc (Linux); elsewhere it finds nothing.
func list() ([]Proc, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(data)
		l, r := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if l < 0 || r < l {
			continue
		}
		f := strings.Fields(s[r+1:])
		if len(f) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		out = append(out, Proc{PID: pid, PPID: ppid, Comm: s[l+1 : r]})
	}
	return out, nil
}

// usage reads /proc/<pid>/stat: utime+stime (clock ticks, 100/s) and rss
// (pages of 4 KiB).
func usage(pid int) (cpu, rss uint64, ok bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, false
	}
	s := string(data)
	r := strings.LastIndexByte(s, ')')
	if r < 0 {
		return 0, 0, false
	}
	f := strings.Fields(s[r+1:])
	if len(f) < 22 {
		return 0, 0, false
	}
	ut, _ := strconv.ParseUint(f[11], 10, 64)
	st, _ := strconv.ParseUint(f[12], 10, 64)
	pages, _ := strconv.ParseUint(f[21], 10, 64)
	return (ut + st) * 10_000_000, pages * 4096, true
}
