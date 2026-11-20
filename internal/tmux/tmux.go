// Package tmux conversa com o servidor tmux do Argus.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Nomes fixos dentro do servidor.
const (
	Session    = "argus"
	MainWindow = "main"
)

// Papéis de painel (opção @argus_role).
const (
	RolePanel       = "panel"
	RolePlaceholder = "placeholder"
	RoleAgent       = "agent"
)

// Client roda comandos num servidor tmux (tmux -L Socket).
type Client struct {
	Socket string
	Bin    string
}

// New cria um cliente; o binário vem do PATH ou dos lugares comuns do Homebrew.
func New(socket string) *Client {
	return &Client{Socket: socket, Bin: findBin()}
}

func findBin() string {
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

// Run roda um comando. Vários comandos podem ir numa chamada só, separados por
// ";" como argumento próprio (um processo só).
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

// HasSession diz se a sessão do Argus existe.
func (c *Client) HasSession() bool {
	_, err := c.Run("has-session", "-t", Session)
	return err == nil
}

// Pane é um painel do tmux com as opções que o Argus grava nele.
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
	Session  string // id da conversa retomada (claude/opencode), se houver
	Worktree string // repositório de origem, se a sessão roda numa worktree
}

// InMain diz se o painel está na janela principal (lista + sessão visível).
func (p Pane) InMain() bool { return p.Window == MainWindow }

var paneFields = []string{
	"#{pane_id}", "#{window_name}", "#{pane_index}", "#{pane_dead}", "#{pane_pid}",
	"#{@argus_role}", "#{@argus_id}", "#{@argus_profile}", "#{@argus_dir}", "#{@argus_name}",
	"#{@argus_created}", "#{@argus_session}", "#{@argus_worktree}",
}

// PaneFormat é o formato usado por ListPanes (campos separados por tab).
var PaneFormat = strings.Join(paneFields, "\t")

// ListPanes lista os painéis da sessão do Argus.
func (c *Client) ListPanes() ([]Pane, error) {
	out, err := c.Run("list-panes", "-s", "-t", Session, "-F", PaneFormat)
	if err != nil {
		return nil, err
	}
	return ParsePanes(out), nil
}

// ParsePanes interpreta a saída de list-panes no formato PaneFormat.
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
		ps = append(ps, Pane{
			ID: f[0], Window: f[1], Index: idx, Dead: f[3] == "1", PID: pid,
			Role: f[5], AgentID: f[6], Profile: f[7], Dir: f[8], Name: f[9],
			Created: created, Session: f[11], Worktree: f[12],
		})
	}
	return ps
}

// SetPaneOptions grava opções de painel num comando só.
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
