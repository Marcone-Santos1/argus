// Package sysmon measures the machine's CPU use and how many processes are
// being created per minute, for the panel footer and the process-storm alarm
// (what once froze the machine with another tool of this kind).
package sysmon

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A rate of StormRate processes/min sustained for StormFor raises the alarm.
const (
	StormRate = 2000
	StormFor  = 30 * time.Second
)

// Reading is one monitor reading.
type Reading struct {
	CPU        float64 // % of the whole machine, 0-100; <0 = no data
	PerMinute  float64 // processes created per minute; <0 = no data
	Storm      bool    // rate above StormRate for StormFor
	StormSince time.Time
}

type sample struct {
	at  time.Time
	pid int
}

// Monitor accumulates readings. On Linux it reads the kernel's last PID; on
// macOS, which does not expose it, each Sample starts /usr/bin/true to learn
// the next PID, and the probes themselves are subtracted from the rate.
type Monitor struct {
	busy, total uint64
	samples     []sample
	stormStart  time.Time
	probes      bool             // whether Probe starts a process (counted out)
	Probe       func() int       // returns a fresh PID; replaceable in tests
	Now         func() time.Time // replaceable in tests
}

// New creates a monitor with the best PID source for this OS.
func New() *Monitor {
	if _, ok := nextPID(); ok {
		return &Monitor{Probe: func() int { n, _ := nextPID(); return n }, Now: time.Now}
	}
	return &Monitor{Probe: probePID, Now: time.Now, probes: true}
}

func probePID() int {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Start(); err != nil {
		return 0
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

// Sample takes a reading. The first call only sets the baseline.
func (m *Monitor) Sample() Reading {
	r := Reading{CPU: -1, PerMinute: -1}
	if b, t, ok := cpuTicks(); ok {
		if m.total != 0 && t > m.total {
			r.CPU = float64(b-m.busy) / float64(t-m.total) * 100
		}
		m.busy, m.total = b, t
	}
	now := m.Now()
	pid := m.Probe()
	if pid > 0 {
		m.samples = append(m.samples, sample{now, pid})
		// A 60 s window.
		for len(m.samples) > 2 && now.Sub(m.samples[0].at) > time.Minute {
			m.samples = m.samples[1:]
		}
		first := m.samples[0]
		if span := now.Sub(first.at); span >= time.Second && pid > first.pid {
			created := float64(pid - first.pid)
			if m.probes {
				created -= float64(len(m.samples) - 1)
			}
			r.PerMinute = max(0, created) / span.Minutes()
		} else if pid < first.pid {
			m.samples = m.samples[len(m.samples)-1:] // PIDs wrapped around
		}
	}
	if r.PerMinute >= StormRate {
		if m.stormStart.IsZero() {
			m.stormStart = now
		}
		if now.Sub(m.stormStart) >= StormFor {
			r.Storm = true
			r.StormSince = m.stormStart
		}
	} else {
		m.stormStart = time.Time{}
	}
	return r
}

// TopCommands lists the programs with the most live processes above sincePID
// (the recently created ones), most frequent first. Runs `ps` once.
func TopCommands(sincePID, n int) []string {
	out, err := exec.Command("ps", "-A", "-o", "pid=,comm=").Output()
	if err != nil {
		return nil
	}
	return topFromPS(string(out), sincePID, n)
}

func topFromPS(out string, sincePID, n int) []string {
	count := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= sincePID {
			continue
		}
		count[filepath.Base(strings.Join(f[1:], " "))]++
	}
	names := make([]string, 0, len(count))
	for k := range count {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if count[names[i]] != count[names[j]] {
			return count[names[i]] > count[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > n {
		names = names[:n]
	}
	return names
}

// FirstPID is the oldest PID in the current window (the base for TopCommands).
func (m *Monitor) FirstPID() int {
	if len(m.samples) == 0 {
		return 0
	}
	return m.samples[0].pid
}
