package ui

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
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

// The code column belongs to the session it was opened in: switching to
// another session hides it, and coming back brings it again.
func (p *Panel) toggleCode() {
	if p.editorPane != "" {
		for _, a := range p.agents {
			if a.Displayed {
				delete(p.codeOpen, a.ID)
			}
		}
		p.hideEditor()
		p.reconcile()
		return
	}
	if a := p.current(); a != nil && !a.Displayed {
		p.swapC = nil
		p.showSelected()
	}
	dir := p.codeDir()
	if dir == "" {
		return
	}
	if a := p.current(); a != nil {
		p.codeOpen[a.ID] = true
	}
	if pane := p.joinEditor(dir); pane != "" {
		_, _ = p.d.Tmux.Run("select-pane", "-t", pane)
	}
}

// joinEditor puts the folder's editor in the code column, between the list
// and the session (half of the right side).
func (p *Panel) joinEditor(dir string) string {
	pane, err := p.editorFor(dir)
	if err != nil {
		p.flash(err.Error())
		return ""
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
	return pane
}

// hideEditor takes the code column off the screen; the editor keeps running.
func (p *Panel) hideEditor() {
	if p.editorPane == "" {
		return
	}
	if _, err := p.d.Tmux.Run("break-pane", "-d", "-s", p.editorPane, "-n", editorWindow(p.editorDir)); err != nil {
		p.flash(err.Error())
	}
	p.editorPane, p.editorDir = "", ""
}

// codeFor shows or hides the code column for the session now on screen.
func (p *Panel) codeFor(a *agent) {
	switch {
	case !p.codeOpen[a.ID]:
		if p.editorPane != "" {
			p.hideEditor()
			p.applyWidth()
		}
	case p.editorPane == "":
		p.joinEditor(a.Dir)
	case p.editorDir != a.Dir:
		p.showEditor(a.Dir)
	}
}

// codeArgs are the tmux commands (each after a ";") that give the session
// now on the right its code column, or take the column away, recording the
// result; codeFor's work for a session switch, in the same call as the swap.
func (p *Panel) codeArgs(a *agent) []string {
	var cmds [][]string
	switch {
	case !p.codeOpen[a.ID]:
		if p.editorPane == "" {
			return nil
		}
		cmds = append(cmds, []string{"break-pane", "-d", "-s", p.editorPane, "-n", editorWindow(p.editorDir)})
		p.editorPane, p.editorDir = "", ""
		if w := p.widthArgs(); w != nil {
			cmds = append(cmds, w)
		}
	case p.editorPane == "":
		pane, err := p.editorFor(a.Dir)
		if err != nil {
			p.flash(err.Error())
			return nil
		}
		cmds = append(cmds, []string{"join-pane", "-h", "-b", "-d", "-l", "50%", "-s", pane, "-t", a.Pane})
		p.editorPane, p.editorDir = pane, a.Dir
		if w := p.widthArgs(); w != nil {
			cmds = append(cmds, w)
		}
	case p.editorDir != a.Dir:
		pane, err := p.editorFor(a.Dir)
		if err != nil {
			p.flash(err.Error())
			return nil
		}
		cmds = append(cmds, []string{"swap-pane", "-d", "-s", pane, "-t", p.editorPane})
		p.editorPane, p.editorDir = pane, a.Dir
	}
	var out []string
	for _, c := range cmds {
		out = append(append(out, ";"), c...)
	}
	return out
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
	if isFresh(cmd) {
		// A named Fresh daemon per folder: the panel can open files in it,
		// and reopening the column finds the same files open.
		cmd = "fresh -a " + freshName(dir)
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
	title := fmt.Sprintf(" #[fg="+hClaude+"]✎ #[fg="+hText+",bold]%s#[nobold] ", strings.ReplaceAll(p.d.Paths.Pretty(dir), "#", "##"))
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

// isFresh: the editor is Fresh run plainly ("fresh" or "fresh ."), so the
// panel manages it (named daemon, follows the agent's edits).
func isFresh(cmd string) bool {
	f := strings.Fields(cmd)
	return len(f) > 0 && filepath.Base(f[0]) == "fresh" && (len(f) == 1 || (len(f) == 2 && f[1] == "."))
}

// freshName is the Fresh daemon of a folder's editor.
func freshName(dir string) string {
	return fmt.Sprintf("argus-%x", sha1.Sum([]byte(dir)))[:14]
}

// followEdit opens, in the code column, the file a session just changed, at
// the changed line: when the column shows that session's folder and you are
// not typing in it. The transcript is read a moment later, once Claude has
// written the change there.
func (p *Panel) followEdit(id string) {
	var a *agent
	for i := range p.agents {
		if p.agents[i].ID == id {
			a = &p.agents[i]
		}
	}
	if a == nil || !isFresh(p.d.Config.Editor) {
		return
	}
	// The folder's editor, shown or not: a hidden one opens the file too, so
	// the column shows the latest change when it comes back.
	pane, open := p.editors[a.Dir]
	if !open {
		return
	}
	path := p.transcriptOf(*a)
	if path == "" {
		return
	}
	name, tm, ch, last := freshName(a.Dir), p.d.Tmux, p.asyncC, p.lastEdit[id]
	go func() {
		time.Sleep(300 * time.Millisecond)
		e, ok := history.LastEdit(path)
		if !ok || e.Key == last {
			return
		}
		// Shown and focused (a hidden editor is alone, so active, in its window).
		if active, err := tm.Run("display", "-p", "-t", pane, "#{?#{==:#{window_name},"+tmux.MainWindow+"},#{pane_active},0}"); err == nil && active == "1" {
			return // you are in the editor: it does not jump under you
		}
		// Only into the folder's own daemon: open-file would start one when
		// missing (an editor opened before this version has none).
		if out, err := exec.Command("fresh", "--cmd", "daemon", "list").Output(); err != nil || !strings.Contains(string(out), name) {
			return
		}
		_ = exec.Command("fresh", "--cmd", "daemon", "open-file", name, fmt.Sprintf("%s:%d", e.File, e.Line)).Run()
		ch <- func() { p.lastEdit[id] = e.Key }
	}()
}

// transcriptOf is a Claude session's transcript ("" when unknown).
func (p *Panel) transcriptOf(a agent) string {
	prof, ok := p.profileOf(a.Profile)
	if !ok || prof.Label() != history.Claude || a.Session == "" {
		return ""
	}
	path, ok := p.transcripts[a.Session]
	if !ok {
		path = history.TranscriptPath(filepath.Join(prof.ClaudeConfigDir(p.d.Paths.Home), "projects"), a.Session)
		if path != "" {
			p.transcripts[a.Session] = path
		}
	}
	return path
}

// saveEditors closes the code column's Fresh editors cleanly when nobody is
// looking at Argus any more: Fresh saves a session's open files only when
// its daemon is told to stop, and a shutdown would kill it without that.
// The next time a column opens, Fresh restores them.
func (p *Panel) saveEditors() {
	if out, err := p.d.Tmux.Run("list-clients", "-t", tmux.Session); err != nil || strings.TrimSpace(out) != "" {
		return // someone is still attached
	}
	if !isFresh(p.d.Config.Editor) || len(p.editors) == 0 {
		return
	}
	for dir := range p.editors {
		_ = exec.Command("fresh", "--cmd", "daemon", "kill", freshName(dir)).Run()
	}
}
