// Package tmux talks to the Argus tmux server.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Fixed names inside the server.
const (
	Session    = "argus"
	MainWindow = "main"
)

// Pane roles (the @argus_role option).
const (
	RolePanel       = "panel"
	RolePlaceholder = "placeholder"
	RoleAgent       = "agent"
	RoleEditor      = "editor" // the code column's editor, one per folder
)

// Client runs commands against one tmux server (tmux -L Socket).
type Client struct {
	Socket string
	Bin    string
}

// New creates a client; the binary comes from PATH or the usual Homebrew places.
func New(socket string) *Client {
	return &Client{Socket: socket, Bin: findBin()}
}

func findBin() string {
	if p, err := exec.LookPath("tmux"); err == nil {
		return p
	}
	for _, c := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "tmux"
}

// Run runs a command. Several commands can share one call, separated by ";"
// as its own argument (one process).
func (c *Client) Run(args ...string) (string, error) {
	cmd := exec.Command(c.Bin, append([]string{"-L", c.Socket}, args...)...)
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

// HasSession reports whether the Argus session exists.
func (c *Client) HasSession() bool {
	_, err := c.Run("has-session", "-t", Session)
	return err == nil
}

// Pane is a tmux pane plus the options Argus stores in it.
type Pane struct {
	ID       string
	Window   string
	Index    int
	Dead     bool
	PID      int
	Role     string // panel | placeholder | agent
	AgentID  string
	Profile  string
	Dir      string
	Name     string
	Created  int64
	Session  string // id of the resumed (or detected) conversation, if any
	Worktree string // source repository when the session runs in a worktree
	Width    int
	Height   int
	Zoomed   bool  // its window has a zoomed pane
	Asleep   int64 // unix time it was hibernated (0 = running)
	Order    int64 // place in the list set by moving it (0 = by creation)
}

// InMain reports whether the pane is in the main window (list + visible session).
func (p Pane) InMain() bool { return p.Window == MainWindow }

var paneFields = []string{
	"#{pane_id}", "#{window_name}", "#{pane_index}", "#{pane_dead}", "#{pane_pid}",
	"#{@argus_role}", "#{@argus_id}", "#{@argus_profile}", "#{@argus_dir}", "#{@argus_name}",
	"#{@argus_created}", "#{@argus_session}", "#{@argus_worktree}",
	"#{pane_width}", "#{pane_height}", "#{window_zoomed_flag}", "#{@argus_sleep}",
	"#{@argus_order}",
}

// PaneFormat is the list-panes format used by ListPanes (tab separated).
var PaneFormat = strings.Join(paneFields, "\t")

// ListPanes lists the panes of the Argus session.
func (c *Client) ListPanes() ([]Pane, error) {
	out, err := c.Run("list-panes", "-s", "-t", Session, "-F", PaneFormat)
	if err != nil {
		return nil, err
	}
	return ParsePanes(out), nil
}

// ParsePanes interprets list-panes output in PaneFormat.
func ParsePanes(out string) []Pane {
	var ps []Pane
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != len(paneFields) {
			continue
		}
		idx, _ := strconv.Atoi(f[2])
		pid, _ := strconv.Atoi(f[4])
		created, _ := strconv.ParseInt(f[10], 10, 64)
		pw, _ := strconv.Atoi(f[13])
		ph, _ := strconv.Atoi(f[14])
		asleep, _ := strconv.ParseInt(f[16], 10, 64)
		order, _ := strconv.ParseInt(f[17], 10, 64)
		ps = append(ps, Pane{
			ID: f[0], Window: f[1], Index: idx, Dead: f[3] == "1", PID: pid,
			Role: f[5], AgentID: f[6], Profile: f[7], Dir: f[8], Name: f[9],
			Created: created, Session: f[11], Worktree: f[12],
			Width: pw, Height: ph, Zoomed: f[15] == "1", Asleep: asleep, Order: order,
		})
	}
	return ps
}

// SetPaneOptions stores pane options in a single command.
func (c *Client) SetPaneOptions(pane string, kv ...string) error {
	var args []string
	for i := 0; i+1 < len(kv); i += 2 {
		if len(args) > 0 {
			args = append(args, ";")
		}
		args = append(args, "set", "-p", "-t", pane, kv[i], kv[i+1])
	}
	if len(args) == 0 {
		return nil
	}
	_, err := c.Run(args...)
	return err
}
