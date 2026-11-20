package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	sessionName = "argus"
	mainWindow  = "main"
)

// socketName: servidor tmux do Argus. ARGUS_SOCKET permite um servidor de teste.
var socketName = func() string {
	if s := os.Getenv("ARGUS_SOCKET"); s != "" {
		return s
	}
	return "argus"
}()

var tmuxBin = findTmux()

func findTmux() string {
	if p, err := exec.LookPath("tmux"); err == nil {
		return p
	}
	for _, c := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "tmux"
}

// tm roda um comando do tmux no servidor do Argus. Vários comandos podem ir
// numa chamada só, separados por ";" (um processo só).
func tm(args ...string) (string, error) {
	cmd := exec.Command(tmuxBin, append([]string{"-L", socketName}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return "", fmt.Errorf("tmux %s: %s", name, strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	return strings.TrimRight(string(out), "\n"), nil
}

type pane struct {
	ID      string
	Window  string
	Index   int
	Dead    bool
	PID     int
	Role    string // panel | placeholder | agent
	AgentID string
	Profile string
	Dir     string
	Name    string
	Created int64
}

func (p pane) inMain() bool { return p.Window == mainWindow }

const paneFmt = "#{pane_id}\t#{window_name}\t#{pane_index}\t#{pane_dead}\t#{pane_pid}\t#{@argus_role}\t#{@argus_id}\t#{@argus_profile}\t#{@argus_dir}\t#{@argus_name}\t#{@argus_created}"

func listPanes() ([]pane, error) {
	out, err := tm("list-panes", "-s", "-t", sessionName, "-F", paneFmt)
	if err != nil {
		return nil, err
	}
	var ps []pane
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 11 {
			continue
		}
		idx, _ := strconv.Atoi(f[2])
		pid, _ := strconv.Atoi(f[4])
		created, _ := strconv.ParseInt(f[10], 10, 64)
		ps = append(ps, pane{
			ID: f[0], Window: f[1], Index: idx, Dead: f[3] == "1", PID: pid,
			Role: f[5], AgentID: f[6], Profile: f[7], Dir: f[8], Name: f[9], Created: created,
		})
	}
	return ps, nil
}

func sessionExists() bool {
	_, err := tm("has-session", "-t", sessionName)
	return err == nil
}
