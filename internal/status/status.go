// Package status lê e grava o estado de cada sessão: uma palavra num arquivo
// (gravado pelo hook do Claude Code, pelo plugin do opencode ou pelo painel).
package status

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Idle    = "idle"    // aberta, sem nada pedido
	Working = "working" // o agente está agindo
	Waiting = "waiting" // pediu permissão ou fez uma pergunta
	Done    = "done"    // respondeu e parou
	Ended   = "ended"   // o processo terminou
)

// Read devolve o estado gravado e quando ele mudou. Sem arquivo: Idle.
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

// Write grava um estado.
func Write(path, s string) error {
	return os.WriteFile(path, []byte(s+"\n"), 0o644)
}

// Opencode é o estado de uma conversa do opencode, gravado pelo plugin do Argus
// como "estado\npasta\n" num arquivo com o id da conversa.
type Opencode struct {
	Session string
	Status  string
	Dir     string
	Changed time.Time
}

// ReadOpencode lê todos os estados gravados pelo plugin.
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

// ReadOpencodeFile lê um arquivo de estado do plugin do opencode.
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

// Short formata uma duração em uma unidade: 12s, 3m, 2h, 5d.
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
