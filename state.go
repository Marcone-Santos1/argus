package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Estados gravados pelo hook (um arquivo por sessão, com uma palavra).
const (
	stIdle    = "idle"
	stWorking = "working"
	stWaiting = "waiting"
	stDone    = "done"
	stEnded   = "ended"
)

func statePath(id string) string { return filepath.Join(stateDir, id) }

func readStatus(id string) (string, time.Time) {
	p := statePath(id)
	fi, err := os.Stat(p)
	if err != nil {
		return stIdle, time.Time{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return stIdle, fi.ModTime()
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		s = stIdle
	}
	return s, fi.ModTime()
}

func writeStatus(id, s string) error {
	return os.WriteFile(statePath(id), []byte(s+"\n"), 0o644)
}

func newID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	default:
		return itoa(int(d.Hours()/24)) + "d"
	}
}

func itoa(n int) string {
	if n < 0 {
		n = 0
	}
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
