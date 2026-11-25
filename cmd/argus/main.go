// Command argus is a terminal panel for running many coding-agent sessions
// (Claude Code, opencode, Gemini CLI, Codex) side by side, on its own tmux server.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/Marcone-Santos1/argus/internal/bench"
	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/install"
	"github.com/Marcone-Santos1/argus/internal/notify"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/sysmon"
	"github.com/Marcone-Santos1/argus/internal/tmux"
	"github.com/Marcone-Santos1/argus/internal/ui"
)

var started = time.Now()

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func versionString() string {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return version
}

func main() {
	socket := os.Getenv("ARGUS_SOCKET")
	if socket == "" {
		socket = "argus"
	}
	p := paths.Default(socket)
	t := tmux.New(socket)

	var err error
	if len(os.Args) < 2 {
		err = launch(p, t)
	} else {
		switch os.Args[1] {
		case "panel":
			err = runPanel(p, t)
		case "install":
			err = install.Install(p, os.Stdout)
			if err == nil && t.HasSession() {
				if _, e := t.Run("source-file", p.TmuxConf()); e == nil {
					fmt.Println(i18n.T("cli.tmuxReloaded"))
				}
			}
		case "uninstall":
			err = install.Uninstall(p, os.Stdout)
			if err == nil && t.HasSession() {
				fmt.Println(i18n.T("cli.serverKept"))
			}
		case "bench":
			fs := flag.NewFlagSet("bench", flag.ExitOnError)
			secs := fs.Int("s", 60, "seconds per CPU measurement")
			_ = fs.Parse(os.Args[2:])
			err = bench.Run(p, t, *secs, os.Stdout)
		case "version", "--version", "-v":
			fmt.Println("argus", versionString())
		case "-h", "--help", "help":
			fmt.Print(i18n.T("cli.usage"))
		default:
			fmt.Fprint(os.Stderr, i18n.T("cli.usage"))
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "argus:", err)
		os.Exit(1)
	}
}

func runPanel(p paths.Paths, t *tmux.Client) error {
	cfg, err := config.Load(p.Config(), p.Home)
	if err != nil {
		return err
	}
	mon := sysmon.New()
	mon.StormRate = float64(cfg.StormRate)
	return ui.New(ui.Deps{
		Paths: p, Config: cfg, Tmux: t, Self: os.Getenv("TMUX_PANE"),
		Notify: notify.Send, Monitor: mon, Started: started,
	}).Run()
}

func selfPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "argus"
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// launch makes sure the server and the panel are alive and replaces this
// process with the tmux client: no Argus process is left outside tmux.
func launch(p paths.Paths, t *tmux.Client) error {
	if _, err := os.Stat(p.TmuxConf()); err != nil {
		return errors.New(i18n.T("cli.needInstall"))
	}
	if strings.Contains(os.Getenv("TMUX"), "/"+t.Socket+",") {
		return errors.New(i18n.T("cli.insideArgus"))
	}
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		return err
	}
	self := selfPath()
	main := tmux.Session + ":" + tmux.MainWindow
	if !t.HasSession() {
		args := []string{"-f", p.TmuxConf(), "new-session", "-d", "-s", tmux.Session, "-n", tmux.MainWindow}
		if t.Socket != "argus" {
			args = append(args, "-e", "ARGUS_SOCKET="+t.Socket)
		}
		args = append(args, self, "panel", ";", "set", "-w", "-t", main, "remain-on-exit", "on")
		if _, err := t.Run(args...); err != nil {
			return err
		}
	} else if err := ensurePanel(t, self); err != nil {
		return err
	}
	// The window always follows the terminal size (a manual resize would pin it).
	_, _ = t.Run("set", "-w", "-u", "-t", main, "window-size", ";",
		"select-window", "-t", main, ";", "select-pane", "-t", main+".0")
	return syscall.Exec(t.Bin, []string{"tmux", "-L", t.Socket, "attach-session", "-t", tmux.Session}, os.Environ())
}

func ensurePanel(t *tmux.Client, self string) error {
	ps, err := t.ListPanes()
	if err != nil {
		return err
	}
	target := ""
	for _, p := range ps {
		if p.Role == tmux.RolePanel {
			if !p.Dead {
				return nil
			}
			target = p.ID
		}
	}
	if target == "" {
		for _, p := range ps {
			if p.InMain() && p.Index == 0 {
				if !p.Dead && p.Role == "" {
					return nil // the panel is starting
				}
				target = p.ID
			}
		}
	}
	if target == "" {
		return errors.New(i18n.T("cli.noPanel"))
	}
	_, err = t.Run("respawn-pane", "-k", "-t", target, self, "panel")
	return err
}
