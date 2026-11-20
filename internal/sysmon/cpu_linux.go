//go:build linux

package sysmon

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// cpuTicks reads the whole machine's CPU counters from /proc/stat.
func cpuTicks() (busy, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	return parseProcStat(sc.Text())
}

// parseProcStat reads the aggregate "cpu" line: user nice system idle iowait irq softirq steal...
func parseProcStat(line string) (busy, total uint64, ok bool) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, false
	}
	var v []uint64
	for _, s := range f[1:] {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		v = append(v, n)
	}
	idle := v[3]
	if len(v) > 4 {
		idle += v[4] // iowait
	}
	for _, n := range v[:min(len(v), 8)] { // guest columns are already in user/nice
		total += n
	}
	return total - idle, total, true
}

// nextPID reads the last PID the kernel handed out, without starting a process.
func nextPID() (int, bool) {
	b, err := os.ReadFile("/proc/sys/kernel/ns_last_pid")
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return n, err == nil
}
