// Package status reads and writes each session's state: one word in a file,
// written by the agent hooks, the opencode plugin or the panel.
package status

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Idle    = "idle"    // open, nothing asked yet
	Working = "working" // the agent is acting
	Waiting = "waiting" // asked for permission or a question
	Done    = "done"    // answered and stopped
	Ended   = "ended"   // the process exited
)

// Read returns the stored state and when it changed. No file means Idle.
func Read(path string) (string, time.Time) {
	fi, err := os.Stat(path)
	if err != nil {
		return Idle, time.Time{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Idle, fi.ModTime()
	}
	s, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	if s == "" {
		s = Idle
	}
	return s, fi.ModTime()
}

// Session returns the conversation id the hook stored on the second line.
func Session(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}

// Subagents is how many subagents the hook saw running (third line).
func Subagents(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(lines[2]))
	return max(n, 0)
}

// Write stores a state.
func Write(path, s string) error {
	return os.WriteFile(path, []byte(s+"\n"), 0o644)
}

// Opencode is the state of one opencode conversation, written by the Argus
// plugin as "state\nfolder\n" in a file named after the conversation id.
type Opencode struct {
	Session string
	Status  string
	Dir     string
	Changed time.Time
}

// ReadOpencode reads every state written by the plugin.
func ReadOpencode(dir string) map[string]Opencode {
	out := map[string]Opencode{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if oc, ok := ReadOpencodeFile(filepath.Join(dir, e.Name())); ok {
			out[oc.Session] = oc
		}
	}
	return out
}

// ReadOpencodeFile reads one state file written by the opencode plugin.
func ReadOpencodeFile(path string) (Opencode, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return Opencode{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Opencode{}, false
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 || lines[0] == "" {
		return Opencode{}, false
	}
	return Opencode{Session: filepath.Base(path), Status: lines[0], Dir: lines[1], Changed: fi.ModTime()}, true
}

// Short formats a duration in one unit: 12s, 3m, 2h, 5d.
func Short(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
