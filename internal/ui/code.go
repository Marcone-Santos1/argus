package ui

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// ---------- the code column (key e): an editor between the list and the session ----------
//
// One editor per folder, started the first time the column opens there.
// Closing the column hides the editor (it keeps running, with its open
// files); switching to a session in another folder brings that folder's
// editor.

// codeDir is the folder for the code column: the selected session's, or the
// selected folder of past conversations.
func (p *Panel) codeDir() string {
	if a := p.current(); a != nil {
		return a.Dir
	}
	if p.sel >= 0 && p.sel < len(p.items) && !strings.HasPrefix(p.items[p.sel].dir, "\x00") {
		return p.items[p.sel].dir
	}
	for _, a := range p.agents {
		if a.Displayed {
			return a.Dir
		}
	}
	return ""
}

func (p *Panel) toggleCode() {
	if p.editorPane != "" {
		if _, err := p.d.Tmux.Run("break-pane", "-d", "-s", p.editorPane, "-n", editorWindow(p.editorDir)); err != nil {
			p.flash(err.Error())
		}
		p.reconcile()
		return
	}
	dir := p.codeDir()
	if dir == "" {
		return
	}
	pane, err := p.editorFor(dir)
	if err != nil {
		p.flash(err.Error())
		return
	}
	// Between the list and the session, half of the right side.
	args := []string{"join-pane", "-h", "-b", "-d", "-l", "50%", "-s", pane, "-t", p.displayPane}
	if p.displayPane == "" {
		args = []string{"join-pane", "-h", "-d", "-s", pane, "-t", p.d.Self}
	}
	if _, err := p.d.Tmux.Run(args...); err != nil {
		p.flash(err.Error())
	}
	p.reconcile()
	p.applyWidth()
	_, _ = p.d.Tmux.Run("select-pane", "-t", pane)
}

// showEditor puts the folder's editor in the code column, in place of the
// one shown.
func (p *Panel) showEditor(dir string) {
	pane, err := p.editorFor(dir)
	if err != nil {
		p.flash(err.Error())
		return
	}
	if _, err := p.d.Tmux.Run("swap-pane", "-d", "-s", pane, "-t", p.editorPane); err != nil {
		p.flash(err.Error())
		return
	}
	p.editorPane, p.editorDir = pane, dir
}

// editorFor is the folder's editor pane, started (hidden) when there is none.
func (p *Panel) editorFor(dir string) (string, error) {
	if pane, ok := p.editors[dir]; ok {
		return pane, nil
	}
	cmd := p.d.Config.Editor
	if f := strings.Fields(cmd); len(f) > 0 {
		if _, err := exec.LookPath(f[0]); err != nil && !strings.Contains(f[0], "/") {
			if _, err2 := os.Stat("/opt/homebrew/bin/" + f[0]); err2 != nil {
				return "", fmt.Errorf("%s", i18n.T("msg.noEditor", f[0]))
			}
		}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	pane, err := p.d.Tmux.Run("new-window", "-d", "-t", tmux.Session+":", "-n", editorWindow(dir), "-c", dir,
		"-P", "-F", "#{pane_id}", shell, "-l", "-i", "-c", "exec "+cmd)
	if err != nil {
		return "", err
	}
	title := fmt.Sprintf(" #[fg=#d97757]✎ #[fg=#d7dae0,bold]%s#[nobold] ", strings.ReplaceAll(p.d.Paths.Pretty(dir), "#", "##"))
	_ = p.d.Tmux.SetPaneOptions(pane, "@argus_role", tmux.RoleEditor, "@argus_dir", dir, "@argus_title", title)
	p.editors[dir] = pane
	return pane, nil
}

// editorWindow names the hidden window of a folder's editor.
func editorWindow(dir string) string {
	return fmt.Sprintf("ed-%x", sha1.Sum([]byte(dir)))[:11]
}

// codeOnlyChoice: a new session (not a resume, an account switch or a
// worktree) can also open just the editor, with no agent.
func (p *Panel) codeOnlyChoice() bool {
	return p.resume == nil && p.switching == "" && p.worktreeOf == "" && p.chosenDir != ""
}

func (p *Panel) profCount() int {
	if p.codeOnlyChoice() {
		return len(p.profChoices) + 1
	}
	return len(p.profChoices)
}

// openCodeOnly opens a folder in the code column, without an agent.
func (p *Panel) openCodeOnly(dir string) {
	p.closePicker()
	p.prefs.Dirs[dir] = config.DirPref{Profile: p.prefs.Dirs[dir].Profile, Last: time.Now().Unix()}
	_ = p.prefs.Save()
	if p.editorPane != "" {
		p.showEditor(dir)
		_, _ = p.d.Tmux.Run("select-pane", "-t", p.editorPane)
		return
	}
	pane, err := p.editorFor(dir)
	if err != nil {
		p.flash(err.Error())
		return
	}
	args := []string{"join-pane", "-h", "-b", "-d", "-l", "50%", "-s", pane, "-t", p.displayPane}
	if p.displayPane == "" {
		args = []string{"join-pane", "-h", "-d", "-s", pane, "-t", p.d.Self}
	}
	if _, err := p.d.Tmux.Run(args...); err != nil {
		p.flash(err.Error())
	}
	p.reconcile()
	p.applyWidth()
	_, _ = p.d.Tmux.Run("select-pane", "-t", pane)
}
