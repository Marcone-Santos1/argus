// Package ui is the Argus panel: the list on the left, which switches the
// session shown on the right. It never polls: it wakes only on a key, a file
// change in the state directory, SIGWINCH and a 2 s clock.
package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/term"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/sysmon"
	"github.com/Marcone-Santos1/argus/internal/tmux"
	"github.com/Marcone-Santos1/argus/internal/usage"
)

// Deps is what the panel uses from the rest of the program.
type Deps struct {
	Paths   paths.Paths
	Config  *config.Config
	Tmux    *tmux.Client
	Self    string // tmux pane the panel runs in ($TMUX_PANE)
	Notify  func(title, body string)
	Monitor *sysmon.Monitor
	Started time.Time
}

type agent struct {
	ID, Pane, Profile, Dir, Name string
	Session                      string // resumed or detected conversation (opencode)
	Worktree                     string // source repository when running in a worktree
	Created                      int64
	Displayed                    bool
	PID                          int // the pane's process (to find its background shells)
	Status                       string
	Since                        time.Time
}

type mode int

const (
	modeList mode = iota
	modePickDir
	modePickProfile
	modeConfirm
	modeHistory
	modeFilter
	modeInput
	modeHelp
	modeAnswer
	modeTemplates
	modeOverview
	modeTimeline
)

// Panel is the panel state.
type Panel struct {
	d     Deps
	prefs *config.Prefs
	hist  *history.Loader

	agents      []agent
	items       []item
	sel         int
	displayPane string
	listFilter  string

	mode       mode
	backMode   mode
	filter     string
	projects   []project
	filtered   []project
	pickSel    int
	chosenDir  string
	profSel    int
	confirmMsg string
	confirmFn  func()
	confirmNo  func() // run when the confirmation is declined

	histAll     []history.Entry
	histShown   []history.Entry
	histSel     int
	resume      *history.Entry
	profChoices []config.Profile
	worktreeOf  string // repository when creating a session in a worktree

	switching string // session being moved to another account (key a)

	restoreOffer []snapEntry       // sessions offered for reopening after a restart
	lastSnap     []byte            // last snapshot written
	snapReady    bool              // the restore decision was made; the snapshot may be written
	prompts      map[string]prompt // what each waiting session asks
	captureC     chan string       // waiting sessions whose screen should be read
	answering    string            // session being answered (key y)

	asyncC       chan func()           // results of background work, applied on the main loop
	git          map[string]gitState   // per folder
	reply        map[string]string     // per session: start of Claude's last reply
	usage        map[string]usage.Info // per session: the status line's last report
	extrasSeen   map[string]bool       // sessions whose git/reply were read once
	shells       map[string]int        // per session: commands still running in the background
	sized        map[string]string     // hidden session pane -> size its window was given
	res          map[string]sessRes    // per session: CPU and memory of its processes
	cpuPrev      map[int]uint64        // CPU time per process at the last reading
	cpuPrevAt    time.Time
	cpuSeries    map[string][]float64 // per session: CPU readings of the last minute
	ovSel        int                  // overview: selected tile
	ov           ovLayout
	screens      map[string][]string // overview: each session's screen
	capturing    bool
	tplFrom      string // session to save as a template (t)
	tplSel       int
	firstPrompt  string // first message of the next session started (templates)
	searchCancel func()
	searching    bool
	contentHits  map[string]string // history search: conversation key -> snippet

	inputPrompt string
	input       string
	inputFn     func(string)

	recentC       chan []history.Entry
	recentLoading bool

	oc        map[string]status.Opencode
	lastState map[string]string // to notify only on changes

	reading sysmon.Reading
	cpuHist []float64 // last CPU readings, for the footer sparkline
	memHist []float64 // last memory readings (% in use)

	expanded      map[string]bool   // folders of past conversations opened in the list
	titleOf       map[string]string // conversation id -> its title (from the history)
	folderW       int               // width of the folder-name column in the list
	hiddenFolders int               // folders with old conversations left out of the list
	prevLines     []string          // what is on screen, to redraw only what changed
	titles        map[string]string // pane title already set, per pane
	stormWarned   bool

	msg      string
	msgUntil time.Time

	w, h       int
	widthDirty bool  // the panel was resized by hand; save the width on the next tick
	rowTarget  []int // per screen line, what a click there selects (-1 = nothing)
	lastClick  time.Time
	lastRow    int
	out        *bufio.Writer
	quit       bool

	swapTimer *time.Timer
	swapC     <-chan time.Time
	swapStart time.Time

	benchAck string
	rendered bool
	metrics  map[string]float64
}

// New builds a panel (without touching the terminal).
func New(d Deps) *Panel {
	if d.Started.IsZero() {
		d.Started = time.Now()
	}
	if d.Notify == nil {
		d.Notify = func(string, string) {}
	}
	accounts := map[string]string{}
	for _, pr := range d.Config.ProfilesFor("claude") {
		if a := history.AccountOf(pr.ClaudeStateFile(d.Paths.Home)); a != "" {
			if _, dup := accounts[a]; !dup {
				accounts[a] = pr.Name
			}
		}
	}
	h := &history.Loader{
		ClaudeProjects: d.Paths.ClaudeProjects(),
		Cache:          d.Paths.HistoryCache(),
		Accounts:       accounts,
	}
	if ps := d.Config.ProfilesFor("opencode"); len(ps) > 0 {
		h.OpencodeDB = d.Paths.OpencodeDB()
		h.OpencodeProfile = ps[0].Name
	}
	return &Panel{
		d: d, prefs: config.LoadPrefs(d.Paths.Prefs()), hist: h,
		recentC: make(chan []history.Entry, 1), metrics: map[string]float64{},
		asyncC: make(chan func(), 16), git: map[string]gitState{}, reply: map[string]string{},
		usage: map[string]usage.Info{}, extrasSeen: map[string]bool{}, shells: map[string]int{}, sized: map[string]string{},
		res: map[string]sessRes{}, cpuPrev: map[int]uint64{}, cpuSeries: map[string][]float64{},
		screens: map[string][]string{},
		prompts: map[string]prompt{}, expanded: map[string]bool{}, titleOf: map[string]string{}, captureC: make(chan string, 8),
		oc: map[string]status.Opencode{}, lastState: map[string]string{}, titles: map[string]string{},
		out: bufio.NewWriterSize(os.Stdout, 32<<10), w: 40, h: 20,
	}
}

// Run takes over the terminal until the user quits.
func (p *Panel) Run() error {
	if p.d.Self == "" {
		return errors.New(i18n.T("cli.panelOutside"))
	}
	for _, d := range []string{p.d.Paths.State, p.d.Paths.Opencode(), p.d.Paths.Usage()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	_ = p.d.Tmux.SetPaneOptions(p.d.Self, "@argus_role", tmux.RolePanel)

	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	// Mouse: clicks and the wheel (SGR encoding); tmux forwards them only
	// because the panel asks for them.
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[?1000h\x1b[?1006h")
	defer func() {
		fmt.Fprint(os.Stdout, "\x1b[?1006l\x1b[?1000l\x1b[?7h\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(os.Stdin.Fd()), old)
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	for _, d := range []string{p.d.Paths.State, p.d.Paths.Opencode(), p.d.Paths.Usage()} {
		if err := watcher.Add(d); err != nil {
			return err
		}
	}

	keys := make(chan string, 64)
	go readKeys(keys)
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	p.resize()
	p.oc = status.ReadOpencode(p.d.Paths.Opencode())
	p.loadUsage()
	p.reconcile()
	p.offerRestore()
	p.render()
	p.refreshRecent()
	if p.d.Monitor != nil {
		p.d.Monitor.Sample()
	}

	for !p.quit {
		select {
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			p.handleKey(k)
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			p.handleFS(ev)
		case <-watcher.Errors:
		case <-ticker.C:
			p.tick()
		case f := <-p.asyncC:
			f()
		case id := <-p.captureC:
			p.capturePrompt(id)
		case r := <-p.recentC:
			p.recentLoading = false
			p.setRecent(r)
		case <-winch:
			p.resize()
			p.noteWidth()
			p.syncHiddenSize(p.displaySize())
		case <-p.swapC:
			p.swapC = nil
			p.showSelected()
		}
		if !p.quit {
			p.render()
		}
	}
	return nil
}

func (p *Panel) tick() {
	if p.widthDirty {
		p.widthDirty = false
		p.prefs.PanelWidth = p.w
		_ = p.prefs.Save()
	}
	if p.msg != "" && time.Now().After(p.msgUntil) {
		p.msg = ""
	}
	if p.d.Monitor == nil {
		return
	}
	p.reading = p.d.Monitor.Sample()
	p.sampleSessions(time.Now())
	if p.mode == modeOverview {
		p.captureScreens()
	}
	if p.reading.CPU >= 0 {
		p.cpuHist = pushLast(p.cpuHist, p.reading.CPU)
	}
	if m := p.reading.MemPercent(); m >= 0 {
		p.memHist = pushLast(p.memHist, m)
	}
	if p.reading.Storm && !p.stormWarned {
		p.stormWarned = true
		top := sysmon.TopCommands(p.d.Monitor.FirstPID(), 1)
		what := ""
		if len(top) > 0 {
			what = i18n.T("msg.stormTop", top[0])
		}
		p.flash(i18n.T("msg.stormFlash", p.reading.PerMinute, what))
		p.d.Notify(i18n.T("notify.stormTitle"), i18n.T("notify.stormBody", p.reading.PerMinute, what))
	} else if !p.reading.Storm {
		p.stormWarned = false
	}
}

// ---------- state ----------

func (p *Panel) reconcile() {
	ps, err := p.d.Tmux.ListPanes()
	if err != nil {
		p.flash("tmux: " + err.Error())
		return
	}
	var placeholders, inMain []tmux.Pane
	var agents []agent
	live := map[string]bool{}
	for _, pn := range ps {
		if pn.ID == p.d.Self {
			continue
		}
		if pn.InMain() {
			inMain = append(inMain, pn)
		}
		switch pn.Role {
		case tmux.RolePlaceholder:
			if !pn.InMain() {
				placeholders = append(placeholders, pn)
			}
		case tmux.RoleAgent:
			if pn.Dead {
				_, _ = p.d.Tmux.Run("kill-pane", "-t", pn.ID)
				continue
			}
			a := agent{ID: pn.AgentID, Pane: pn.ID, Profile: pn.Profile, Dir: pn.Dir, Name: pn.Name,
				Session: pn.Session, Worktree: pn.Worktree, Created: pn.Created, Displayed: pn.InMain(), PID: pn.PID}
			agents = append(agents, a)
			live[a.ID] = true
		}
	}

	// The right side always shows something: the chosen session or the empty space.
	if len(inMain) == 0 {
		if len(placeholders) > 0 {
			ph := placeholders[0]
			placeholders = placeholders[1:]
			if _, err := p.d.Tmux.Run("join-pane", "-h", "-d", "-l", "70%", "-s", ph.ID, "-t", p.d.Self); err == nil {
				p.displayPane = ph.ID
				p.applyWidth()
			}
		} else if id, err := p.d.Tmux.Run("split-window", "-h", "-d", "-l", "70%", "-t", p.d.Self, "-P", "-F", "#{pane_id}", "true"); err == nil {
			_ = p.d.Tmux.SetPaneOptions(id, "@argus_role", tmux.RolePlaceholder)
			p.displayPane = id
			p.applyWidth()
		}
	} else {
		p.displayPane = inMain[0].ID
		if d := inMain[0]; !d.Zoomed {
			defer p.syncHiddenSize(d.Width, d.Height)
		}
		for _, extra := range inMain[1:] {
			if extra.Role == tmux.RolePlaceholder {
				_, _ = p.d.Tmux.Run("kill-pane", "-t", extra.ID)
			}
		}
	}
	// Hidden empty spaces (swapped with a session) are useless.
	for _, extra := range placeholders {
		_, _ = p.d.Tmux.Run("kill-pane", "-t", extra.ID)
	}

	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Created < agents[j].Created })
	key := p.selKey()
	p.agents = agents
	for i := range p.agents {
		p.refreshStatus(i, false)
	}
	p.rebuildItems(key)
	fresh := false
	for i := range p.agents {
		if a := p.agents[i]; !p.extrasSeen[a.ID] {
			p.extrasSeen[a.ID] = true
			p.refreshExtras(a)
			fresh = true
		}
	}
	if fresh {
		p.checkShells()
	}
	for i := range p.agents {
		p.syncTitle(i)
	}
	p.saveSnapshot()
	for id := range p.lastState {
		if !live[id] {
			p.logState(id, "", stClosed, time.Now())
			delete(p.lastState, id)
		}
	}
	for id := range p.usage {
		if !live[id] {
			delete(p.usage, id)
			delete(p.reply, id)
			_ = os.Remove(filepath.Join(p.d.Paths.Usage(), id))
		}
	}

	// State files of sessions that no longer exist.
	if entries, err := os.ReadDir(p.d.Paths.State); err == nil {
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || strings.HasPrefix(n, "_") || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "bench") || live[n] {
				continue
			}
			_ = os.Remove(filepath.Join(p.d.Paths.State, n))
		}
	}
}

func (p *Panel) profileOf(name string) (config.Profile, bool) { return p.d.Config.Profile(name) }

func (p *Panel) isOpencode(a agent) bool {
	pr, ok := p.profileOf(a.Profile)
	return ok && pr.Label() == "opencode"
}

// refreshStatus re-reads a session's state; notifies (when asked) on a change.
func (p *Panel) refreshStatus(i int, notify bool) {
	a := &p.agents[i]
	st, since := status.Read(p.d.Paths.Status(a.ID))
	if sid := status.Session(p.d.Paths.Status(a.ID)); sid != "" && sid != a.Session {
		// The hook saw the conversation id (new session, /resume or /clear).
		a.Session = sid
		_ = p.d.Tmux.SetPaneOptions(a.Pane, "@argus_session", sid)
	}
	if p.isOpencode(*a) {
		if oc, ok := p.opencodeFor(a); ok && oc.Changed.After(since) {
			st, since = oc.Status, oc.Changed
		}
	}
	prev, seen := p.lastState[a.ID]
	a.Status, a.Since = st, since
	p.lastState[a.ID] = st
	if !seen || prev != st {
		p.logState(a.ID, p.sessionText(*a), st, since)
	}
	if seen && prev == status.Working && st != status.Working {
		p.refreshRecent() // the turn ended: the conversation may have a (new) title
		p.refreshExtras(*a)
		p.checkShells()
	}
	if st == status.Waiting && (prev != st || !seen) {
		p.capturePromptSoon(a.ID)
	} else if st != status.Waiting {
		delete(p.prompts, a.ID)
	}
	if notify && seen && prev != st && p.d.Config.ShouldNotify(st) && !p.focusedOn(*a) {
		title := "Argus: " + a.Name
		body := map[string]string{status.Waiting: i18n.T("notify.waiting"), status.Done: i18n.T("notify.done")}[st]
		if body == "" {
			body = st
		}
		p.d.Notify(title, body)
	}
}

// opencodeFor finds a session's opencode conversation: by id when the session
// was resumed; otherwise the newest conversation in the same folder started
// after the session (and not claimed by another one). The id found is stored
// in the tmux pane so the association does not change later.
func (p *Panel) opencodeFor(a *agent) (status.Opencode, bool) {
	if a.Session != "" {
		oc, ok := p.oc[a.Session]
		return oc, ok
	}
	claimed := map[string]bool{}
	for _, o := range p.agents {
		if o.Session != "" {
			claimed[o.Session] = true
		}
	}
	created := time.Unix(0, a.Created).Add(-2 * time.Second)
	var best status.Opencode
	found := false
	for _, oc := range p.oc {
		if oc.Dir != a.Dir || claimed[oc.Session] || oc.Changed.Before(created) {
			continue
		}
		if !found || oc.Changed.After(best.Changed) {
			best, found = oc, true
		}
	}
	if found {
		a.Session = best.Session
		_ = p.d.Tmux.SetPaneOptions(a.Pane, "@argus_session", best.Session)
	}
	return best, found
}

// focusedOn reports whether the user is looking at the session right now.
func (p *Panel) focusedOn(a agent) bool {
	if !a.Displayed {
		return false
	}
	out, err := p.d.Tmux.Run("display", "-p", "-t", tmux.Session+":"+tmux.MainWindow, "#{session_attached}\t#{pane_id}")
	if err != nil {
		return false
	}
	att, active, _ := strings.Cut(out, "\t")
	return att != "0" && active == a.Pane
}

func (p *Panel) handleFS(ev fsnotify.Event) {
	name := filepath.Base(ev.Name)
	if filepath.Dir(ev.Name) == p.d.Paths.Usage() {
		if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
			p.readUsage(ev.Name)
		}
		return
	}
	if filepath.Dir(ev.Name) == p.d.Paths.Opencode() {
		if oc, ok := status.ReadOpencodeFile(ev.Name); ok {
			p.oc[oc.Session] = oc
			for i := range p.agents {
				if p.isOpencode(p.agents[i]) {
					p.refreshStatus(i, true)
					p.syncTitle(i)
				}
			}
		}
		return
	}
	switch {
	case name == "_tmux":
		if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
			p.reconcile()
			p.refreshRecent()
		}
	case name == "_bench":
		if data, err := os.ReadFile(ev.Name); err == nil && len(data) > 0 {
			p.benchAck = string(data)
		}
	case strings.HasPrefix(name, "_") || strings.HasPrefix(name, "."):
	default:
		for i := range p.agents {
			if p.agents[i].ID == name {
				p.refreshStatus(i, true)
				p.syncTitle(i)
			}
		}
	}
}

func (p *Panel) flash(s string) {
	p.msg = s
	p.msgUntil = time.Now().Add(5 * time.Second)
}

func (p *Panel) saveMetrics() {
	data, _ := json.Marshal(p.metrics)
	_ = os.WriteFile(p.d.Paths.Metrics(), data, 0o644)
}

// syncTitle keeps the title tmux draws above a session up to date. It costs
// one tmux call, and only when the text actually changes.
func (p *Panel) syncTitle(i int) {
	a := p.agents[i]
	icon, color, word := statusStyle(a.Status)
	if p.shells[a.ID] > 0 {
		icon, color = "◜", cWorking
	}
	esc := func(s string) string { return strings.ReplaceAll(s, "#", "##") }
	title := fmt.Sprintf(" #[fg=%s]%s#[default] #[bold]%s#[nobold]  #[fg=%s,bold]%s#[nobold]#[fg=#6c717c] · %s · #[fg=%s]%s ",
		tmuxColor(color), icon, esc(p.sessionText(a)),
		p.accountHex(a.Profile), esc(p.tag(a.Profile)),
		esc(p.d.Paths.Pretty(a.Dir)), tmuxColor(color), i18n.T(word))
	if n := p.shells[a.ID]; n > 0 {
		title += fmt.Sprintf("#[fg=%s]· %s ", tmuxColor(cWorking), i18n.T("list.shells", n))
	}
	if r, ok := p.res[a.ID]; ok && r.RSS > 0 {
		title += "#[fg=#6c717c]· " + size(r.RSS) + " "
	}
	if in, ok := p.profileUsage(a.Profile); ok && !in.FiveHour.Expired(time.Now()) {
		c := pctColor(in.FiveHour.Percent)
		if c == "" {
			c = cQuiet
		}
		title += fmt.Sprintf("#[fg=#6c717c]· 5h #[fg=%s]%.0f%% ", tmuxColor(c), in.FiveHour.Percent)
	}
	if p.titles[a.Pane] == title {
		return
	}
	if err := p.d.Tmux.SetPaneOptions(a.Pane, "@argus_title", title); err == nil {
		p.titles[a.Pane] = title
	}
}
