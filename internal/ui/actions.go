package ui

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
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
	if a := p.current(); a != nil && !a.Asleep.IsZero() {
		if !p.wake(a) {
			return
		}
		p.refreshStatus(p.items[p.sel].agent, false)
	}
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
	p.stopSearch()
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
	pane, dir := a.Pane, a.Dir
	initial := a.Name
	if autoName(a.Name, a.Dir) {
		initial = ""
	}
	p.ask(i18n.T("msg.renamePrompt"), initial, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			name = filepath.Base(dir) // empty = back to the profile's text
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
	prompt := p.firstPrompt // a template's first message, for this session only
	p.firstPrompt = ""
	wt := p.worktreeOf
	if p.awake() >= p.d.Config.MaxSessions {
		p.closePicker()
		p.confirm(i18n.T("msg.limit", p.awake(), p.d.Config.MaxSessions),
			func() { p.firstPrompt = prompt; p.createAgent(dir, prof, resume, wt) })
		return
	}
	p.firstPrompt = prompt
	p.createAgent(dir, prof, resume, wt)
}

// launch returns the tmux -e options and the command for starting a profile
// in a session: the user's login shell (same PATH and aliases as a terminal)
// running the profile's command, with the resume arguments when resuming.
// Variables other profiles set and this one does not are unset, so switching
// accounts never inherits the previous account's CLAUDE_CONFIG_DIR.
func (p *Panel) launch(id string, prof config.Profile, resume *history.Entry) (env, cmd []string) {
	env = []string{"-e", "ARGUS_ID=" + id}
	for k, v := range prof.Env {
		env = append(env, "-e", k+"="+v)
	}
	line := ""
	unset := map[string]bool{}
	for _, other := range p.d.Config.Profiles {
		for k := range other.Env {
			if _, mine := prof.Env[k]; !mine && !unset[k] {
				unset[k] = true
				line += "unset " + k + "; "
			}
		}
	}
	line += "exec " + prof.Command
	if resume != nil {
		for _, a := range history.ResumeArgs(resume.Tool, resume.ID) {
			line += " " + shellQuote(a)
		}
	} else if p.firstPrompt != "" {
		for _, a := range history.PromptArgs(prof.Label(), p.firstPrompt) {
			line += " " + shellQuote(a)
		}
		p.firstPrompt = ""
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	return env, []string{shell, "-l", "-i", "-c", line}
}

// shellQuote protects an argument for a shell command line.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (p *Panel) createAgent(dir string, prof config.Profile, resume *history.Entry, wtRepo string) {
	p.closePicker()
	p.worktreeOf = ""
	id, err := p.spawn(dir, prof, resume, wtRepo, "")
	if err != nil {
		p.flash(err.Error())
		if id == "" {
			return
		}
	}
	prefDir := dir
	if wtRepo != "" {
		prefDir = wtRepo
	}
	p.prefs.Dirs[prefDir] = config.DirPref{Profile: prof.Name, Last: time.Now().Unix()}
	if resume != nil {
		p.prefs.Sessions[resume.Key()] = prof.Name
	}
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
	p.histAll = p.withRemembered(p.hist.Load())
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
	p.histAll = p.withRemembered(all)
	for _, e := range p.histAll {
		if e.Title != "" {
			p.titleOf[e.ID] = e.Title
		}
	}
	p.rebuildItems(key)
	for i := range p.agents {
		p.syncTitle(i)
	}
}

// withRemembered applies the profile each conversation was last opened with
// in Argus (an account switch, or the answer to "which account?"), over the
// account the transcript recorded when the conversation started.
func (p *Panel) withRemembered(es []history.Entry) []history.Entry {
	for i := range es {
		if name, ok := p.prefs.Sessions[es[i].Key()]; ok {
			if _, exists := p.d.Config.Profile(name); exists {
				es[i].Profile = name
			}
		}
	}
	return es
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

// applyHistFilter shows the conversations whose title, folder or account
// match, then the ones found by their text (when a search has finished).
func (p *Panel) applyHistFilter() {
	p.histShown = p.histShown[:0]
	for _, e := range p.histAll {
		if matchTerms(e.Dir+" "+e.Title+" "+e.Profile+" "+e.Tool, p.filter) {
			p.histShown = append(p.histShown, e)
		}
	}
	for _, e := range p.histAll {
		if p.isContentHit(e) {
			p.histShown = append(p.histShown, e)
		}
	}
	p.histSel = clamp(p.histSel, len(p.histShown))
}

func (p *Panel) applyFilter() {
	if isPathQuery(p.filter) {
		p.filtered = browseDirs(p.filter, p.d.Paths.Home, maxBrowse)
		p.pickSel = clamp(p.pickSel, len(p.filtered))
		return
	}
	p.filtered = p.filtered[:0]
	for _, pr := range p.projects {
		if matchTerms(pr.Rel, p.filter) {
			p.filtered = append(p.filtered, pr)
		}
	}
	p.pickSel = clamp(p.pickSel, len(p.filtered))
}

// maxBrowse is how many folders a typed path lists.
const maxBrowse = 300

// isPathQuery: text starting with / or ~ browses the disk instead of
// filtering the known projects.
func isPathQuery(q string) bool {
	return strings.HasPrefix(q, "/") || strings.HasPrefix(q, "~")
}

// browseDirs lists the folders a typed path points at: the path itself when
// it is a folder (and ends in /), then the subfolders of its parent that
// start with the last part (case-insensitive). Hidden ones only show when
// the last part starts with a dot.
func browseDirs(q, home string, limit int) []project {
	full := q
	if full == "~" || strings.HasPrefix(full, "~/") {
		full = home + full[1:]
	}
	dir, part := full, ""
	if !strings.HasSuffix(full, "/") {
		dir, part = filepath.Dir(full), filepath.Base(full)
	}
	pretty := func(path string) string {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return "~/"
			}
			return "~/" + rel + "/"
		}
		return strings.TrimSuffix(path, "/") + "/"
	}
	var out []project
	if part == "" {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			out = append(out, project{Path: filepath.Clean(dir), Rel: pretty(filepath.Clean(dir))})
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	sort.SliceStable(ents, func(i, j int) bool { return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name()) })
	low := strings.ToLower(part)
	for _, e := range ents {
		n := e.Name()
		full := filepath.Join(dir, n)
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(part, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(n), low) || !isDir(full, e) {
			continue
		}
		out = append(out, project{Path: full, Rel: pretty(full)})
		if len(out) >= limit {
			break
		}
	}
	return out
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

// switchAccount reopens the selected session's conversation under another
// profile of the same program (another Claude account, for example). The
// process is restarted in the same pane with --resume, so the conversation,
// folder and place in the list stay the same.
func (p *Panel) switchAccount() {
	a := p.current()
	if a == nil {
		return
	}
	cur, ok := p.profileOf(a.Profile)
	if !ok {
		return
	}
	var choices []config.Profile
	for _, pr := range p.d.Config.ProfilesFor(cur.Label()) {
		if pr.Name != a.Profile {
			choices = append(choices, pr)
		}
	}
	if len(choices) == 0 {
		p.flash(i18n.T("msg.noOtherAccount", cur.Label()))
		return
	}
	if a.Session == "" || !history.SafeID(a.Session) {
		p.flash(i18n.T("msg.noSessionID"))
		return
	}
	p.switching = a.ID
	p.chosenDir = a.Dir
	p.resume = nil
	p.worktreeOf = ""
	p.backMode = modeList
	p.askProfile(choices, choices[0].Name)
}

func (p *Panel) doSwitch(prof config.Profile) {
	id := p.switching
	p.switching = ""
	p.closePicker()
	var a *agent
	for i := range p.agents {
		if p.agents[i].ID == id {
			a = &p.agents[i]
		}
	}
	if a == nil {
		return
	}
	target := *a
	do := func() {
		cur, _ := p.profileOf(target.Profile)
		resume := &history.Entry{Tool: cur.Label(), ID: target.Session}
		env, cmd := p.launch(target.ID, prof, resume)
		args := append([]string{"respawn-pane", "-k", "-t", target.Pane, "-c", target.Dir}, env...)
		if _, err := p.d.Tmux.Run(append(args, cmd...)...); err != nil {
			p.flash(err.Error())
			return
		}
		_ = p.d.Tmux.SetPaneOptions(target.Pane, "@argus_profile", prof.Name, "@argus_session", target.Session)
		p.prefs.Sessions[resume.Key()] = prof.Name
		_ = p.prefs.Save()
		_ = os.WriteFile(p.d.Paths.Status(target.ID), []byte(status.Idle+"\n"+target.Session+"\n"), 0o644)
		p.flash(i18n.T("msg.switched", target.Name, prof.Name))
		p.reconcile()
		p.selectKey("a:" + target.ID)
		p.focusSession()
	}
	if target.Status == status.Working || target.Status == status.Waiting {
		p.confirm(i18n.T("msg.switchActive", target.Name), do)
		return
	}
	do()
}

// diffScript shows a folder's changes in less: the short status (new files
// included) and the diff against HEAD. $1 folder, $2 "not a repository",
// $3 "no changes".
const diffScript = `cd "$1" 2>/dev/null || exit 0
{
	if ! git rev-parse --git-dir >/dev/null 2>&1; then
		printf '%s\n' "$2"
	elif [ -z "$(git status --porcelain)" ]; then
		printf '%s\n' "$3"
	else
		git -c color.status=always status --short
		echo
		git -c color.ui=always diff HEAD 2>/dev/null || git -c color.ui=always diff
	fi
} | less -R -K`

// showDiff opens, over the screen, what changed in the selected session's
// (or folder's) directory. q closes it.
func (p *Panel) showDiff() {
	if p.sel < 0 || p.sel >= len(p.items) {
		return
	}
	dir := p.items[p.sel].dir
	if dir == "" {
		return
	}
	title := " " + i18n.T("diff.title", p.folderName(dir)) + " "
	if _, err := p.d.Tmux.Run("display-popup", "-E", "-w", "90%", "-h", "90%", "-T", title, "-d", dir,
		"sh", "-c", diffScript, "sh", dir, i18n.T("diff.notGit"), i18n.T("diff.clean")); err != nil {
		p.flash(err.Error())
	}
}
