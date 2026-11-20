package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
	"github.com/Marcone-Santos1/argus/internal/worktree"
)

func newID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (p *Panel) scheduleSwap() {
	if p.swapTimer != nil {
		p.swapTimer.Stop()
	}
	p.swapTimer = time.NewTimer(80 * time.Millisecond)
	p.swapC = p.swapTimer.C
}

// current is the selected open session (nil on a past conversation).
func (p *Panel) current() *agent {
	if p.sel < 0 || p.sel >= len(p.items) || p.items[p.sel].agent < 0 {
		return nil
	}
	return &p.agents[p.items[p.sel].agent]
}

// showSelected puts the selected session on the right (swapping with the current one).
func (p *Panel) showSelected() bool {
	a := p.current()
	if a == nil {
		return false
	}
	if a.Displayed {
		return true
	}
	p.swapStart = time.Now()
	if _, err := p.d.Tmux.Run("swap-pane", "-d", "-s", a.Pane, "-t", p.displayPane); err != nil {
		p.flash(err.Error())
		p.reconcile()
		return false
	}
	for i := range p.agents {
		p.agents[i].Displayed = false
	}
	a.Displayed = true
	p.displayPane = a.Pane
	return true
}

func (p *Panel) focusSession() {
	p.swapC = nil
	if p.showSelected() {
		_, _ = p.d.Tmux.Run("select-pane", "-t", p.displayPane)
	}
}

// hideSession takes the session off the right without closing it (it keeps running).
func (p *Panel) hideSession() {
	p.swapC = nil
	for _, a := range p.agents {
		if a.Displayed {
			if _, err := p.d.Tmux.Run("break-pane", "-d", "-s", a.Pane, "-n", a.ID); err != nil {
				p.flash(err.Error())
			}
			break
		}
	}
	p.reconcile()
}

func (p *Panel) killSelected() {
	a := p.current()
	if a == nil {
		return
	}
	target := *a
	do := func() {
		if _, err := p.d.Tmux.Run("kill-pane", "-t", target.Pane); err != nil {
			p.flash(err.Error())
			return
		}
		p.flash(i18n.T("msg.closed", target.Name))
		p.reconcile()
		if target.Worktree != "" {
			p.confirm(i18n.T("msg.deleteWorktree", filepath.Base(target.Dir)), func() {
				if err := worktree.Remove(target.Worktree, target.Dir); err != nil {
					p.flash(err.Error())
				} else {
					p.flash(i18n.T("msg.worktreeDeleted"))
				}
				p.refreshRecent()
			})
		}
	}
	if target.Status == status.Working || target.Status == status.Waiting {
		p.confirm(i18n.T("msg.active", target.Name), do)
		return
	}
	do()
}

func (p *Panel) confirm(msg string, fn func()) {
	p.mode = modeConfirm
	p.confirmMsg = msg
	p.confirmFn = fn
}

func (p *Panel) ask(prompt, initial string, fn func(string)) {
	p.mode = modeInput
	p.inputPrompt = prompt
	p.input = initial
	p.inputFn = fn
}

func (p *Panel) setZoom(on bool) {
	out, err := p.d.Tmux.Run("display", "-p", "-t", p.d.Self, "#{window_zoomed_flag}")
	if err != nil {
		return
	}
	if (out == "1") != on {
		_, _ = p.d.Tmux.Run("resize-pane", "-Z", "-t", p.d.Self)
	}
}

func (p *Panel) openPicker() {
	p.mode = modePickDir
	p.filter = ""
	p.pickSel = 0
	p.worktreeOf = ""
	p.resume = nil
	p.projects = scanProjects(p.d.Config, p.prefs, p.d.Paths.Home)
	p.applyFilter()
	p.setZoom(true)
}

func (p *Panel) closePicker() {
	p.mode = modeList
	p.setZoom(false)
}

// nextAttention selects the next session waiting for you (or, if none, the
// next one that finished), starting from the current selection.
func (p *Panel) nextAttention() {
	for _, want := range []string{status.Waiting, status.Done} {
		n := len(p.items)
		for step := 1; step <= n; step++ {
			i := (p.sel + step) % n
			if it := p.items[i]; it.agent >= 0 && p.agents[it.agent].Status == want {
				p.sel = i
				p.swapC = nil
				p.showSelected()
				return
			}
		}
	}
	p.flash(i18n.T("msg.noAttention"))
}

// startWorktree asks for a branch and starts a session in a new worktree of
// the selected folder's repository.
func (p *Panel) startWorktree() {
	if p.sel < 0 || p.sel >= len(p.items) {
		p.flash(i18n.T("msg.pickRepo"))
		return
	}
	repo, err := worktree.RepoRoot(p.items[p.sel].dir)
	if err != nil {
		p.flash(err.Error())
		return
	}
	p.ask(i18n.T("msg.branchPrompt", filepath.Base(repo)), "argus-"+newID(), func(branch string) {
		branch = strings.TrimSpace(branch)
		path, err := worktree.Create(repo, branch)
		if err != nil {
			p.flash(err.Error())
			return
		}
		p.worktreeOf = repo
		p.resume = nil
		p.chosenDir = path
		p.backMode = modeList
		p.askProfile(p.d.Config.Profiles, p.d.Config.DefaultProfile(repo, p.prefs))
	})
}

func (p *Panel) renameSelected() {
	a := p.current()
	if a == nil {
		return
	}
	pane := a.Pane
	p.ask(i18n.T("msg.renamePrompt"), a.Name, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if err := p.d.Tmux.SetPaneOptions(pane, "@argus_name", name); err != nil {
			p.flash(err.Error())
		}
		p.reconcile()
	})
}

func (p *Panel) askProfile(choices []config.Profile, def string) {
	p.profChoices = choices
	p.profSel = 0
	for i, pr := range choices {
		if pr.Name == def {
			p.profSel = i
		}
	}
	p.mode = modePickProfile
}

// startAgent creates the session, honoring the limit of simultaneous sessions.
func (p *Panel) startAgent(dir string, prof config.Profile, resume *history.Entry) {
	wt := p.worktreeOf
	if len(p.agents) >= p.d.Config.MaxSessions {
		p.closePicker()
		p.confirm(i18n.T("msg.limit", len(p.agents), p.d.Config.MaxSessions),
			func() { p.createAgent(dir, prof, resume, wt) })
		return
	}
	p.createAgent(dir, prof, resume, wt)
}

// shellQuote protects an argument for a shell command line.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (p *Panel) createAgent(dir string, prof config.Profile, resume *history.Entry, wtRepo string) {
	p.closePicker()
	p.worktreeOf = ""
	command := prof.Command
	session := ""
	if resume != nil {
		if !history.SafeID(resume.ID) {
			p.flash(i18n.T("msg.badID"))
			return
		}
		for _, a := range history.ResumeArgs(resume.Tool, resume.ID) {
			command += " " + shellQuote(a)
		}
		session = resume.ID
	}
	id := newID()
	name := filepath.Base(dir)
	taken := map[string]bool{}
	for _, a := range p.agents {
		taken[a.Name] = true
	}
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s %d", filepath.Base(dir), n)
	}
	_ = status.Write(p.d.Paths.Status(id), status.Idle)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	args := []string{"new-window", "-d", "-t", tmux.Session + ":", "-n", id, "-c", dir, "-e", "ARGUS_ID=" + id}
	for k, v := range prof.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, "-P", "-F", "#{pane_id}", shell, "-l", "-i", "-c", "exec "+command)
	paneID, err := p.d.Tmux.Run(args...)
	if err != nil {
		p.flash(err.Error())
		return
	}
	if err := p.d.Tmux.SetPaneOptions(paneID,
		"@argus_role", tmux.RoleAgent, "@argus_id", id, "@argus_profile", prof.Name,
		"@argus_dir", dir, "@argus_name", name, "@argus_created", fmt.Sprint(time.Now().UnixNano()),
		"@argus_session", session, "@argus_worktree", wtRepo, "remain-on-exit", "off"); err != nil {
		p.flash(err.Error())
	}
	prefDir := dir
	if wtRepo != "" {
		prefDir = wtRepo
	}
	p.prefs.Dirs[prefDir] = config.DirPref{Profile: prof.Name, Last: time.Now().Unix()}
	_ = p.prefs.Save()

	p.reconcile()
	p.selectKey("a:" + id)
	p.focusSession()
}

// resumeEntry resumes a conversation; it asks for the account only when none is recorded.
func (p *Panel) resumeEntry(e history.Entry) {
	p.backMode = p.mode
	p.resume = &e
	p.chosenDir = e.Dir
	p.worktreeOf = ""
	if prof, ok := p.d.Config.Profile(e.Profile); ok {
		p.startAgent(e.Dir, prof, &e)
		return
	}
	choices := p.d.Config.ProfilesFor(e.Tool)
	if len(choices) == 0 {
		p.flash(i18n.T("msg.noProfile", e.Tool))
		return
	}
	p.askProfile(choices, choices[0].Name)
}

// ---------- history ----------

func (p *Panel) openHistory() {
	p.mode = modeHistory
	p.filter = ""
	p.histSel = 0
	p.resume = nil
	p.setZoom(true)
	t := time.Now()
	p.histAll = p.hist.Load()
	p.metrics["history_ms"] = float64(time.Since(t).Microseconds()) / 1000
	p.saveMetrics()
	p.applyHistFilter()
	p.rebuildItems(p.selKey())
}

// refreshRecent re-reads the history off the main loop (the index keeps it near 10 ms).
func (p *Panel) refreshRecent() {
	if p.recentLoading {
		return
	}
	p.recentLoading = true
	h, ch := p.hist, p.recentC
	go func() { ch <- h.Load() }()
}

func (p *Panel) setRecent(all []history.Entry) {
	key := p.selKey()
	p.histAll = all
	p.rebuildItems(key)
}

func matchTerms(hay, filter string) bool {
	hay = strings.ToLower(hay)
	for _, t := range strings.Fields(strings.ToLower(filter)) {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func (p *Panel) applyHistFilter() {
	p.histShown = p.histShown[:0]
	for _, e := range p.histAll {
		if matchTerms(e.Dir+" "+e.Title+" "+e.Profile+" "+e.Tool, p.filter) {
			p.histShown = append(p.histShown, e)
		}
	}
	p.histSel = clamp(p.histSel, len(p.histShown))
}

func (p *Panel) applyFilter() {
	p.filtered = p.filtered[:0]
	for _, pr := range p.projects {
		if matchTerms(pr.Rel, p.filter) {
			p.filtered = append(p.filtered, pr)
		}
	}
	p.pickSel = clamp(p.pickSel, len(p.filtered))
}

func clamp(i, n int) int {
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}
