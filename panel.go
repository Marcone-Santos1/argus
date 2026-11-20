package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/term"
)

// O painel não tem polling: acorda só com tecla, mudança de arquivo no
// diretório de estado, SIGWINCH e um relógio de 2 s para os tempos na lista.

type agent struct {
	ID, Pane, Profile, Dir, Name string
	Created                      int64
	Displayed                    bool
	Status                       string
	Since                        time.Time
}

type project struct {
	Path, Rel string
	Last      int64
}

type mode int

const (
	modeList mode = iota
	modePickDir
	modePickProfile
	modeConfirm
	modeHistory
)

type panel struct {
	cfg   *Config
	prefs *Prefs
	self  string // pane do painel

	agents      []agent
	sel         int
	displayPane string

	mode       mode
	filter     string
	projects   []project
	filtered   []project
	pickSel    int
	chosenDir  string
	profSel    int
	confirmMsg string
	confirmFn  func()
	zoomed     bool

	history     []histEntry
	histShown   []histEntry
	histSel     int
	resume      *histEntry
	profChoices []Profile

	histAll       []histEntry // conversas antigas, mais recentes primeiro
	items         []item      // o que a lista mostra, já agrupado por pasta
	recentC       chan []histEntry
	recentLoading bool
	backMode      mode

	msg      string
	msgUntil time.Time

	w, h int
	out  *bufio.Writer
	quit bool

	swapTimer *time.Timer
	swapC     <-chan time.Time
	swapStart time.Time

	benchAck string
	rendered bool
	metrics  map[string]float64
}

func runPanel() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	p := &panel{
		cfg: cfg, prefs: loadPrefs(), self: os.Getenv("TMUX_PANE"),
		out: bufio.NewWriterSize(os.Stdout, 32<<10), metrics: map[string]float64{},
		recentC: make(chan []histEntry, 1),
	}
	if p.self == "" {
		return fmt.Errorf("o painel roda dentro do tmux do Argus; use `argus`")
	}
	_, _ = tm("set", "-p", "-t", p.self, "@argus_role", "panel")

	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l\x1b[?7l")
	defer func() {
		fmt.Fprint(os.Stdout, "\x1b[?7h\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(os.Stdin.Fd()), old)
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add(stateDir); err != nil {
		return err
	}

	keys := make(chan string, 64)
	go readKeys(keys)
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	p.resize()
	p.reconcile()
	p.render()
	p.refreshRecent()

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
			if p.msg != "" && time.Now().After(p.msgUntil) {
				p.msg = ""
			}
		case <-winch:
			p.resize()
		case r := <-p.recentC:
			p.recentLoading = false
			p.setRecent(r)
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

// ---------- estado ----------

func (p *panel) reconcile() {
	ps, err := listPanes()
	if err != nil {
		p.flash("tmux: " + err.Error())
		return
	}
	var placeholders []pane
	var inMain []pane
	var agents []agent
	live := map[string]bool{}
	for _, pn := range ps {
		if pn.ID == p.self {
			continue
		}
		if pn.inMain() {
			inMain = append(inMain, pn)
		}
		switch pn.Role {
		case "placeholder":
			if !pn.inMain() {
				placeholders = append(placeholders, pn)
			}
		case "agent":
			if pn.Dead {
				_, _ = tm("kill-pane", "-t", pn.ID)
				continue
			}
			a := agent{ID: pn.AgentID, Pane: pn.ID, Profile: pn.Profile, Dir: pn.Dir, Name: pn.Name, Created: pn.Created, Displayed: pn.inMain()}
			a.Status, a.Since = readStatus(a.ID)
			agents = append(agents, a)
			live[a.ID] = true
		}
	}

	// À direita sempre há algo: a sessão escolhida ou o espaço vazio.
	if len(inMain) == 0 {
		if len(placeholders) > 0 {
			ph := placeholders[0]
			placeholders = placeholders[1:]
			if _, err := tm("join-pane", "-h", "-d", "-l", "70%", "-s", ph.ID, "-t", p.self); err == nil {
				p.displayPane = ph.ID
			}
		} else if id, err := tm("split-window", "-h", "-d", "-l", "70%", "-t", p.self, "-P", "-F", "#{pane_id}", "true"); err == nil {
			_, _ = tm("set", "-p", "-t", id, "@argus_role", "placeholder")
			p.displayPane = id
		}
	} else {
		p.displayPane = inMain[0].ID
		for _, extra := range inMain[1:] {
			if extra.Role == "placeholder" {
				_, _ = tm("kill-pane", "-t", extra.ID)
			}
		}
	}
	// Espaços vazios escondidos (trocados com uma sessão) não servem para nada.
	for _, extra := range placeholders {
		_, _ = tm("kill-pane", "-t", extra.ID)
	}

	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Created < agents[j].Created })
	key := p.selKey()
	p.agents = agents
	p.rebuildItems(key)

	// Arquivos de estado de sessões que já não existem.
	if entries, err := os.ReadDir(stateDir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || strings.HasPrefix(n, "_") || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "bench") || live[n] {
				continue
			}
			_ = os.Remove(filepath.Join(stateDir, n))
		}
	}
}

func (p *panel) groupOf(profile string) string {
	if pr, ok := p.cfg.profile(profile); ok {
		return pr.Group
	}
	return "OUTROS"
}

func (p *panel) handleFS(ev fsnotify.Event) {
	name := filepath.Base(ev.Name)
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
				p.agents[i].Status, p.agents[i].Since = readStatus(name)
			}
		}
	}
}

func (p *panel) flash(s string) {
	p.msg = s
	p.msgUntil = time.Now().Add(4 * time.Second)
}

// ---------- ações ----------

func (p *panel) scheduleSwap() {
	if p.swapTimer != nil {
		p.swapTimer.Stop()
	}
	p.swapTimer = time.NewTimer(80 * time.Millisecond)
	p.swapC = p.swapTimer.C
}

func (p *panel) current() *agent {
	if p.sel < 0 || p.sel >= len(p.items) || p.items[p.sel].agent < 0 {
		return nil
	}
	return &p.agents[p.items[p.sel].agent]
}

// showSelected põe a sessão selecionada no lado direito (troca de lugar com a atual).
func (p *panel) showSelected() bool {
	a := p.current()
	if a == nil {
		return false
	}
	if a.Displayed {
		return true
	}
	p.swapStart = time.Now()
	if _, err := tm("swap-pane", "-d", "-s", a.Pane, "-t", p.displayPane); err != nil {
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

func (p *panel) focusSession() {
	p.swapC = nil
	if p.showSelected() {
		_, _ = tm("select-pane", "-t", p.displayPane)
	}
}

// hideSession tira a sessão da direita sem encerrá-la (ela continua rodando).
func (p *panel) hideSession() {
	p.swapC = nil
	for _, a := range p.agents {
		if a.Displayed {
			if _, err := tm("break-pane", "-d", "-s", a.Pane, "-n", a.ID); err != nil {
				p.flash(err.Error())
			}
			break
		}
	}
	p.reconcile()
}

func (p *panel) killSelected() {
	a := p.current()
	if a == nil {
		return
	}
	pane, name := a.Pane, a.Name
	do := func() {
		if _, err := tm("kill-pane", "-t", pane); err != nil {
			p.flash(err.Error())
		} else {
			p.flash("encerrada: " + name)
		}
		p.reconcile()
	}
	if a.Status == stWorking || a.Status == stWaiting {
		p.confirm(fmt.Sprintf("%s está ativa. Encerrar? (s/n)", name), do)
		return
	}
	do()
}

func (p *panel) confirm(msg string, fn func()) {
	p.mode = modeConfirm
	p.confirmMsg = msg
	p.confirmFn = fn
}

func (p *panel) setZoom(on bool) {
	out, err := tm("display", "-p", "-t", p.self, "#{window_zoomed_flag}")
	if err != nil {
		return
	}
	if (out == "1") != on {
		_, _ = tm("resize-pane", "-Z", "-t", p.self)
	}
	p.zoomed = on
}

func (p *panel) openPicker() {
	p.mode = modePickDir
	p.filter = ""
	p.pickSel = 0
	p.projects = scanProjects(p.cfg, p.prefs)
	p.applyFilter()
	p.setZoom(true)
}

func (p *panel) closePicker() {
	p.mode = modeList
	p.setZoom(false)
}

func (p *panel) applyFilter() {
	terms := strings.Fields(strings.ToLower(p.filter))
	p.filtered = p.filtered[:0]
	for _, pr := range p.projects {
		rel := strings.ToLower(pr.Rel)
		ok := true
		for _, t := range terms {
			if !strings.Contains(rel, t) {
				ok = false
				break
			}
		}
		if ok {
			p.filtered = append(p.filtered, pr)
		}
	}
	if p.pickSel >= len(p.filtered) {
		p.pickSel = len(p.filtered) - 1
	}
	if p.pickSel < 0 {
		p.pickSel = 0
	}
}

func (p *panel) createAgent(dir string, prof Profile, resume *histEntry) {
	p.closePicker()
	command := prof.Command
	if resume != nil {
		if !safeID.MatchString(resume.ID) {
			p.flash("id de conversa inválido")
			return
		}
		command += resumeArgs(resume.Tool, resume.ID)
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
	_ = writeStatus(id, stIdle)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	args := []string{"new-window", "-d", "-t", sessionName + ":", "-n", id, "-c", dir, "-e", "ARGUS_ID=" + id}
	for k, v := range prof.Env {
		args = append(args, "-e", k+"="+expand(v))
	}
	args = append(args, "-P", "-F", "#{pane_id}", shell, "-l", "-i", "-c", "exec "+command)
	paneID, err := tm(args...)
	if err != nil {
		p.flash(err.Error())
		return
	}
	set := func(k, v string) []string { return []string{"set", "-p", "-t", paneID, k, v, ";"} }
	var opts []string
	opts = append(opts, set("@argus_role", "agent")...)
	opts = append(opts, set("@argus_id", id)...)
	opts = append(opts, set("@argus_profile", prof.Name)...)
	opts = append(opts, set("@argus_dir", dir)...)
	opts = append(opts, set("@argus_name", name)...)
	opts = append(opts, set("@argus_created", fmt.Sprint(time.Now().UnixNano()))...)
	opts = append(opts, "set", "-p", "-t", paneID, "remain-on-exit", "off")
	if _, err := tm(opts...); err != nil {
		p.flash(err.Error())
	}

	p.prefs.Dirs[dir] = dirPref{Profile: prof.Name, Last: time.Now().Unix()}
	p.prefs.save()

	p.reconcile()
	p.selectKey("a:" + id)
	p.focusSession()
}

func (p *panel) handleKey(k string) {
	switch p.mode {
	case modeConfirm:
		switch k {
		case "s", "S", "y", "Y", "enter":
			p.mode = modeList
			if p.confirmFn != nil {
				p.confirmFn()
			}
		default:
			p.mode = modeList
		}
		p.confirmFn = nil
		return

	case modePickDir:
		switch k {
		case "esc", "ctrl-C":
			p.closePicker()
		case "up", "ctrl-P":
			if p.pickSel > 0 {
				p.pickSel--
			}
		case "down", "ctrl-N":
			if p.pickSel < len(p.filtered)-1 {
				p.pickSel++
			}
		case "backspace":
			if p.filter != "" {
				_, size := utf8.DecodeLastRuneInString(p.filter)
				p.filter = p.filter[:len(p.filter)-size]
				p.applyFilter()
			}
		case "ctrl-U":
			p.filter = ""
			p.applyFilter()
		case "enter":
			if len(p.filtered) == 0 {
				return
			}
			p.chosenDir = p.filtered[p.pickSel].Path
			p.resume = nil
			p.askProfile(p.cfg.Profiles, p.cfg.defaultProfile(p.chosenDir, p.prefs))
		default:
			if utf8.RuneCountInString(k) == 1 && k >= " " {
				p.filter += k
				p.pickSel = 0
				p.applyFilter()
			}
		}
		return

	case modeHistory:
		p.handleHistoryKey(k)
		return

	case modePickProfile:
		switch k {
		case "esc", "ctrl-C":
			if p.resume != nil && p.backMode == modeHistory {
				p.mode = modeHistory
			} else if p.resume != nil {
				p.closePicker()
			} else {
				p.mode = modePickDir
			}
		case "up", "k":
			if p.profSel > 0 {
				p.profSel--
			}
		case "down", "j":
			if p.profSel < len(p.profChoices)-1 {
				p.profSel++
			}
		case "enter":
			p.startAgent(p.chosenDir, p.profChoices[p.profSel], p.resume)
		default:
			if k >= "1" && k <= "9" && len(k) == 1 {
				if i := int(k[0] - '1'); i < len(p.profChoices) {
					p.profSel = i
				}
			}
		}
		return
	}

	switch k {
	case "up", "k":
		if p.sel > 0 {
			p.sel--
			if p.current() != nil {
				p.scheduleSwap()
			}
		}
	case "down", "j":
		if p.sel < len(p.items)-1 {
			p.sel++
			if p.current() != nil {
				p.scheduleSwap()
			}
		}
	case "enter":
		if p.sel >= 0 && p.sel < len(p.items) && p.items[p.sel].agent < 0 {
			p.resumeEntry(p.items[p.sel].entry)
			return
		}
		p.focusSession()
	case "n":
		p.openPicker()
	case "h":
		p.openHistory()
	case "x":
		p.killSelected()
	case "esc":
		p.hideSession()
	case "z":
		if p.current() != nil {
			p.focusSession()
			_, _ = tm("resize-pane", "-Z", "-t", p.displayPane)
		}
	case "q":
		p.swapC = nil
		_, _ = tm("detach-client", "-s", sessionName)
		p.quit = true
	default:
		if len(k) == 1 && k >= "1" && k <= "9" {
			if it := p.nthAgentItem(int(k[0] - '1')); it >= 0 {
				p.sel = it
				p.swapC = nil
				p.showSelected()
			}
		}
	}
}

// ---------- projetos ----------

func scanProjects(cfg *Config, prefs *Prefs) []project {
	home := homeDir()
	seen := map[string]bool{}
	var out []project
	add := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		rel, err := filepath.Rel(home, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = path
		}
		out = append(out, project{Path: path, Rel: rel, Last: prefs.Dirs[path].Last})
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" || !isDir(filepath.Join(dir, n), e) {
				continue
			}
			full := filepath.Join(dir, n)
			add(full)
			if depth > 1 && !exists(filepath.Join(full, ".git")) {
				walk(full, depth-1)
			}
		}
	}
	for _, r := range cfg.Roots {
		walk(expand(r), cfg.Depth)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Last != out[j].Last {
			return out[i].Last > out[j].Last
		}
		return strings.ToLower(out[i].Rel) < strings.ToLower(out[j].Rel)
	})
	return out
}

func isDir(path string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink != 0 {
		fi, err := os.Stat(path)
		return err == nil && fi.IsDir()
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---------- desenho ----------

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	reverse = "\x1b[7m"
)

func statusIcon(s string) (string, string) {
	switch s {
	case stWorking:
		return "●", "\x1b[33m"
	case stWaiting:
		return "◐", "\x1b[1;35m"
	case stDone:
		return "✓", "\x1b[32m"
	case stEnded:
		return "✕", "\x1b[31m"
	default:
		return "○", "\x1b[2m"
	}
}

func (p *panel) resize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 10 || h < 4 {
		w, h = 40, 20
	}
	p.w, p.h = w, h
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n <= w {
		return s + strings.Repeat(" ", w-n)
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func (p *panel) render() {
	var lines []string
	switch p.mode {
	case modePickDir:
		lines = p.renderPicker()
	case modePickProfile:
		lines = p.renderProfiles()
	case modeHistory:
		lines = p.renderHistory()
	default:
		lines = p.renderList()
	}
	b := p.out
	b.WriteString("\x1b[H")
	for i := 0; i < p.h; i++ {
		if i > 0 {
			b.WriteString("\r\n")
		}
		// Limpa a linha antes de escrever: limpar depois apagaria a última
		// coluna (sem quebra automática o cursor fica sobre ela).
		b.WriteString("\x1b[2K")
		if i < len(lines) {
			b.WriteString(lines[i])
		}
		b.WriteString(reset)
	}
	_ = b.Flush()

	if !p.rendered {
		p.rendered = true
		p.metrics["startup_ms"] = float64(time.Since(startTime).Microseconds()) / 1000
		p.saveMetrics()
	}
	if !p.swapStart.IsZero() {
		p.metrics["switch_ms"] = float64(time.Since(p.swapStart).Microseconds()) / 1000
		p.swapStart = time.Time{}
		p.saveMetrics()
	}
	if p.benchAck != "" {
		_ = os.WriteFile(filepath.Join(stateDir, "_bench.ack"), []byte(p.benchAck), 0o644)
		p.benchAck = ""
	}
}

func (p *panel) saveMetrics() {
	data, _ := json.Marshal(p.metrics)
	_ = os.WriteFile(filepath.Join(stateDir, "_metrics"), data, 0o644)
}

func (p *panel) renderList() []string {
	w := p.w
	body := []string{bold + fit(" argus", w) + reset}
	if len(p.agents) == 0 {
		body = append(body, dim+fit(" Nenhuma sessão aberta. n cria uma;", w)+reset, dim+fit(" Enter numa conversa a retoma.", w)+reset)
	}
	focusLine := -1
	now := time.Now()
	dir := ""
	num := 0
	for idx, it := range p.items {
		if it.dir != dir {
			dir = it.dir
			body = append(body, "", p.folderHeader(dir, w))
		}
		sel := idx == p.sel
		if sel {
			focusLine = len(body)
		}
		if it.agent >= 0 {
			num++
			body = append(body, p.agentRow(p.agents[it.agent], num, sel, now, w))
		} else {
			body = append(body, p.recentRow(it.entry, sel, now, w))
		}
	}

	var foot []string
	if p.mode == modeConfirm {
		foot = append(foot, "\x1b[1;33m"+fit(" "+p.confirmMsg, w)+reset)
	} else if p.msg != "" {
		foot = append(foot, "\x1b[36m"+fit(" "+p.msg, w)+reset)
	}
	foot = append(foot, dim+strings.Repeat("─", w)+reset)
	sess := "sessões"
	if len(p.agents) == 1 {
		sess = "sessão"
	}
	foot = append(foot, fit(fmt.Sprintf(" %d %s", len(p.agents), sess), w))
	hints := []string{"↑↓ ver", "⏎ entrar", "^\\ alterna", "n nova", "h histórico", "esc esconde", "x encerra", "z tela cheia", "q sai"}
	foot = append(foot, wrapHints(hints, w)...)
	return layout(body, foot, p.h, max(focusLine, 0))
}

func wrapHints(hints []string, w int) []string {
	var lines []string
	cur := ""
	for _, h := range hints {
		cand := cur + "  " + h
		if cur == "" {
			cand = " " + h
		}
		if utf8.RuneCountInString(cand) > w && cur != "" {
			lines = append(lines, dim+fit(cur, w)+reset)
			cand = " " + h
		}
		cur = cand
	}
	if cur != "" {
		lines = append(lines, dim+fit(cur, w)+reset)
	}
	return lines
}

// layout junta corpo e rodapé na altura da tela, rolando o corpo se preciso.
func layout(body, foot []string, h, focus int) []string {
	room := h - len(foot)
	if room < 1 {
		room = 1
	}
	if len(body) > room {
		start := focus - room/2
		if start < 0 {
			start = 0
		}
		if start > len(body)-room {
			start = len(body) - room
		}
		body = body[start : start+room]
	}
	out := append([]string{}, body...)
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, foot...)
}

func (p *panel) renderPicker() []string {
	w := p.w
	body := []string{
		bold + fit(" Nova sessão: escolha o projeto", w) + reset,
		"",
		fit(" > "+p.filter+"█", w),
		"",
	}
	if len(p.filtered) == 0 {
		body = append(body, dim+fit(" nada encontrado", w)+reset)
	}
	for i, pr := range p.filtered {
		mark := "  "
		if pr.Last > 0 {
			mark = "· "
		}
		line := fit(" "+mark+pr.Rel, w)
		if i == p.pickSel {
			line = reverse + line + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"digite para filtrar", "↑↓ escolher", "⏎ ok", "esc cancela", "· usado antes"}, w)...)
	return layout(body, foot, p.h, p.pickSel+4)
}

func (p *panel) renderProfiles() []string {
	w := p.w
	rel := p.chosenDir
	if r, err := filepath.Rel(homeDir(), rel); err == nil {
		rel = "~/" + r
	}
	body := []string{
		bold + fit(" "+p.profTitle(), w) + reset,
		dim + fit(" "+rel, w) + reset,
		"",
	}
	for i, pr := range p.profChoices {
		line := fit(fmt.Sprintf(" %d  %-16s %s", i+1, pr.Name, dim+pr.Group+reset), w+len(dim)+len(reset))
		if i == p.profSel {
			line = reverse + fit(fmt.Sprintf(" %d  %-16s %s", i+1, pr.Name, pr.Group), w) + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"↑↓ escolher", "⏎ abrir", "esc voltar"}, w)...)
	return layout(body, foot, p.h, p.profSel+3)
}

// ---------- teclado ----------

func readKeys(ch chan<- string) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			close(ch)
			return
		}
		for _, k := range parseKeys(buf[:n]) {
			ch <- k
		}
	}
}

func parseKeys(b []byte) []string {
	var ks []string
	for i := 0; i < len(b); {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				if j < len(b) {
					switch b[j] {
					case 'A':
						ks = append(ks, "up")
					case 'B':
						ks = append(ks, "down")
					}
					i = j + 1
					continue
				}
			}
			ks = append(ks, "esc")
			i++
			continue
		}
		switch {
		case c == '\r' || c == '\n':
			ks = append(ks, "enter")
		case c == 0x7f || c == 0x08:
			ks = append(ks, "backspace")
		case c == '\t':
			ks = append(ks, "tab")
		case c < 0x20:
			ks = append(ks, "ctrl-"+string(rune(c+'@')))
		default:
			r, size := utf8.DecodeRune(b[i:])
			ks = append(ks, string(r))
			i += size
			continue
		}
		i++
	}
	return ks
}
