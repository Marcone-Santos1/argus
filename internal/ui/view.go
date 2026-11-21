package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[1;31m"
	yellow = "\x1b[1;33m"
	cyan   = "\x1b[36m"
)

func (p *Panel) resize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 10 || h < 4 {
		w, h = 40, 20
	}
	p.w, p.h = w, h
}

// fit cuts or pads with spaces to w columns.
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

func (p *Panel) render() {
	var lines []string
	switch p.mode {
	case modePickDir:
		lines = p.viewPicker()
	case modePickProfile:
		lines = p.viewProfiles()
	case modeHistory:
		lines = p.viewHistory()
	case modeHelp:
		lines = p.viewHelp()
	default:
		lines = p.viewList()
	}
	// Redraw only the lines that changed since the last frame.
	if len(p.prevLines) != p.h {
		p.prevLines = make([]string, p.h)
		for i := range p.prevLines {
			p.prevLines[i] = "\x00" // forces the first full draw
		}
	}
	b := p.out
	for i := 0; i < p.h; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		if l == p.prevLines[i] {
			continue
		}
		p.prevLines[i] = l
		// Clear the line before writing: clearing after would erase the last
		// column (without autowrap the cursor stays on it).
		fmt.Fprintf(b, "\x1b[%d;1H\x1b[2K%s%s", i+1, l, reset)
	}
	_ = b.Flush()

	if !p.rendered {
		p.rendered = true
		p.metrics["startup_ms"] = float64(time.Since(p.d.Started).Microseconds()) / 1000
		p.saveMetrics()
	}
	if !p.swapStart.IsZero() {
		p.metrics["switch_ms"] = float64(time.Since(p.swapStart).Microseconds()) / 1000
		p.swapStart = time.Time{}
		p.saveMetrics()
	}
	if p.benchAck != "" {
		_ = os.WriteFile(filepath.Join(p.d.Paths.State, "_bench.ack"), []byte(p.benchAck), 0o644)
		p.benchAck = ""
	}
}

// Width of the CPU sparkline (one block per 2 s reading).
const sparkLen = 8

func (p *Panel) viewList() []string {
	w := p.w
	var body []string
	if p.mode == modeFilter || p.listFilter != "" {
		cursor := ""
		if p.mode == modeFilter {
			cursor = "█"
		}
		body = append(body, line(w, "", sp(" / ", cAccent), sp(p.listFilter+cursor, "")), "")
	}
	focusLine := -1
	now := time.Now()
	num := 0
	if len(p.agents) > 0 || p.listFilter == "" {
		body = append(body, section(i18n.T("sec.open"), w))
	}
	if len(p.agents) == 0 && p.listFilter == "" {
		body = append(body, line(w, "", sp(i18n.T("list.empty1"), cQuiet)), line(w, "", sp(i18n.T("list.empty2"), cQuiet)))
	}
	recents := false
	p.folderW = 0
	for _, it := range p.items {
		if it.folder {
			p.folderW = max(p.folderW, width(p.folderName(it.dir)))
		}
	}
	p.folderW = min(p.folderW, max(w/3, 8))
	for idx, it := range p.items {
		sel := idx == p.sel && p.mode != modeFilter
		if it.agent < 0 && !recents {
			recents = true
			body = append(body, "", section(i18n.T("sec.recent"), w))
		}
		if idx == p.sel {
			focusLine = len(body)
		}
		switch {
		case it.agent >= 0:
			num++
			a := p.agents[it.agent]
			body = append(body, p.agentRows(a, num, sel, now, w)...)
			if pr, ok := p.prompts[a.ID]; ok && a.Status == status.Waiting {
				body = append(body, p.promptLines(pr, w)...)
			}
		case it.folder:
			body = append(body, p.folderRow(it, sel, now, w))
		default:
			body = append(body, p.entryRow(it.entry, sel, now, w))
		}
	}
	if p.hiddenFolders > 0 && p.listFilter == "" {
		body = append(body, line(w, "", sp(i18n.T("list.more", p.hiddenFolders), cFaint)))
	}
	if len(p.items) == 0 && p.listFilter != "" {
		body = append(body, line(w, "", sp(i18n.T("list.nothing"), cQuiet)))
	}
	p.syncPanelTitle()
	return layout(body, p.footer(), p.h, max(focusLine, 0))
}

// section is a part's heading: a small label and a rule to the edge.
func section(label string, w int) string {
	return line(w, "", sp(" "+label+" ", "\x1b[1m"+cQuiet), sp(strings.Repeat("─", max(w-width(label)-2, 0)), cFaint))
}

// panelTitle is the text tmux draws on the panel's top border: the name and,
// on its right, how many sessions are in each state.
func (p *Panel) panelTitle() string {
	counts := map[string]int{}
	for _, a := range p.agents {
		counts[a.Status]++
	}
	t := " #[fg=#7aa2f7,bold]argus#[nobold] "
	for _, st := range []string{status.Working, status.Waiting, status.Done} {
		if counts[st] == 0 {
			continue
		}
		icon, color, _ := statusStyle(st)
		t += fmt.Sprintf(" #[fg=%s]%s %d", tmuxColor(color), icon, counts[st])
	}
	total := i18n.T("list.sessions", len(p.agents))
	if len(p.agents) == 1 {
		total = i18n.T("list.session")
	}
	return t + " #[fg=#6c717c]· " + total + " #[default]"
}

// syncPanelTitle updates the panel's border title, only when it changes.
func (p *Panel) syncPanelTitle() {
	t := p.panelTitle()
	if p.titles[p.d.Self] == t || p.d.Self == "" {
		return
	}
	if err := p.d.Tmux.SetPaneOptions(p.d.Self, "@argus_title", t); err == nil {
		p.titles[p.d.Self] = t
	}
}

func (p *Panel) footer() []string {
	w := p.w
	var foot []string
	switch {
	case p.mode == modeConfirm:
		foot = append(foot, line(w, "", sp(" "+p.confirmMsg, yellow)))
	case p.mode == modeAnswer:
		foot = append(foot, p.answerLines(w)...)
	case p.mode == modeInput:
		foot = append(foot, line(w, "", sp(" "+p.inputPrompt, yellow)), line(w, "", sp(" > "+p.input+"█", "")))
	case p.msg != "":
		foot = append(foot, line(w, "", sp(" "+p.msg, cyan)))
	}
	if p.reading.Storm {
		foot = append(foot, line(w, "", sp(i18n.T("list.storm", p.reading.PerMinute), red)))
	}
	foot = append(foot, line(w, "", sp(strings.Repeat("─", w), cFaint)))
	if stats := p.statsSpans(); stats != nil {
		foot = append(foot, line(w, "", stats...))
	}
	for _, h := range wrapHints(tr(p.hintKeys()...), w) {
		foot = append(foot, line(w, "", sp(stripANSI(h), cFaint)))
	}
	return foot
}

// hintKeys are the keys that make sense for what is selected.
func (p *Panel) hintKeys() []string {
	if p.sel >= 0 && p.sel < len(p.items) {
		it := p.items[p.sel]
		switch {
		case it.agent >= 0:
			keys := []string{"hint.enter", "hint.toggle"}
			if p.agents[it.agent].Status == status.Waiting {
				keys = append(keys, "hint.answer")
			}
			return append(keys, "hint.rename", "hint.close", "hint.help")
		case it.folder:
			return []string{"hint.expand", "hint.new", "hint.history", "hint.help"}
		default:
			return []string{"hint.resume", "hint.new", "hint.help"}
		}
	}
	return []string{"hint.new", "hint.history", "hint.help"}
}

// autoName reports whether a session still has the name Argus gave it: the
// folder name, or "folder 2", "folder 3"... for more sessions in the folder.
func autoName(name, dir string) bool {
	base := filepath.Base(dir)
	if name == "" || name == base {
		return true
	}
	rest, ok := strings.CutPrefix(name, base+" ")
	if !ok || rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// sessionText is what names a session: the name given with r, otherwise the
// conversation's title, otherwise "new conversation".
func (p *Panel) sessionText(a agent) string {
	if !autoName(a.Name, a.Dir) {
		return a.Name
	}
	if t := p.titleOf[a.Session]; t != "" && a.Session != "" {
		return t
	}
	return i18n.T("list.newConv")
}

// tag is a profile's short account label ("emp", "oc").
func (p *Panel) tag(profile string) string {
	if pr, ok := p.profileOf(profile); ok {
		return pr.Tag()
	}
	return "?"
}

// rowBar is the column every row starts with: blue on the selection, amber
// on a session waiting for you.
func rowBar(sel, waiting bool) (bg string, bar span) {
	switch {
	case sel:
		return bgSel, sp("▌", cAccent)
	case waiting:
		return bgWait, sp("▌", cAmber)
	}
	return "", sp(" ", "")
}

// agentRows is an open session in two lines: status, number, title and
// state; under it, the account and the folder.
func (p *Panel) agentRows(a agent, num int, sel bool, now time.Time, w int) []string {
	icon, color, word := statusStyle(a.Status)
	el := ""
	if !a.Since.IsZero() {
		el = status.Short(now.Sub(a.Since))
	}
	right := i18n.T(word) + " " + el
	if w < 44 { // narrow: no status word
		right = el
	}
	tag := p.tag(a.Profile)
	n := " "
	if num <= 9 {
		n = fmt.Sprint(num)
	}
	textStyle := ""
	if a.Displayed || sel {
		textStyle = "\x1b[1m"
	}
	bg, bar := rowBar(sel, a.Status == status.Waiting)
	used := 1 + 3 + 3 // bar + icon + number
	textW := max(w-used-width(right)-2, 1)
	first := line(w, bg, bar,
		sp(" "+icon+" ", color), sp(n+"  ", cQuiet),
		sp(fit(p.sessionText(a), textW), textStyle), sp("  ", ""),
		sp(right, color), sp(" ", ""))
	where := p.d.Paths.Pretty(a.Dir)
	if a.Worktree != "" {
		where += " ⑂"
	}
	second := line(w, bg, bar, sp("      ", ""), sp(tag, p.accountColor(a.Profile)), sp(" · "+where, cFaint))
	return []string{first, second}
}

// folderRow is a folder of past conversations: name, how many, the newest
// title and its age. ▸ / ▾ tell whether it is open.
func (p *Panel) folderRow(it item, sel bool, now time.Time, w int) string {
	bg, bar := rowBar(sel, false)
	arrow := "▸"
	if p.expanded[it.dir] || p.listFilter != "" {
		arrow = "▾"
	}
	right := fmt.Sprintf("%2d · %3s", it.count, status.Short(now.Sub(it.entry.Updated)))
	name := fit(p.folderName(it.dir), max(p.folderW, 1))
	nameStyle := cFolder
	if !sel {
		nameStyle = "\x1b[38;2;97;175;239m" // the folder color without bold
	}
	titleW := max(w-1-3-width(name)-2-width(right)-3, 0)
	title := strings.Repeat(" ", titleW)
	if titleW >= 6 && arrow == "▸" { // open, its conversations show below
		title = fit(it.entry.Title, titleW)
	}
	return line(w, bg, bar, sp(" "+arrow+" ", cFaint), sp(name, nameStyle), sp("  ", ""),
		sp(title, cFaint), sp("  ", ""), sp(right, cFaint), sp(" ", ""))
}

func (p *Panel) folderName(dir string) string {
	if p.d.Paths.Pretty(dir) == "~" {
		return "~"
	}
	return filepath.Base(dir)
}

// entryRow is a past conversation inside an open folder.
func (p *Panel) entryRow(e history.Entry, sel bool, now time.Time, w int) string {
	bg, bar := rowBar(sel, false)
	tag := p.entryTag(e)
	age := fmt.Sprintf("%3s", status.Short(now.Sub(e.Updated)))
	titleW := max(w-1-5-width(tag)-2-width(age)-3, 1)
	titleStyle := cQuiet
	if sel {
		titleStyle = ""
	}
	return line(w, bg, bar, sp("     ", ""), sp(fit(e.Title, titleW), titleStyle), sp("  ", ""),
		sp(tag, p.accountColor(e.Profile)), sp("  ", ""), sp(age, cFaint), sp(" ", ""))
}

func (p *Panel) entryTag(e history.Entry) string {
	if e.Tool == history.Opencode {
		return "oc"
	}
	if pr, ok := p.profileOf(e.Profile); ok {
		return pr.Tag()
	}
	return "?"
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

// layout fits body and footer to the screen height, scrolling the body so the
// focus line stays visible.
func layout(body, foot []string, h, focus int) []string {
	room := max(h-len(foot), 1)
	if len(body) > room {
		start := min(max(focus-room/2, 0), len(body)-room)
		body = body[start : start+room]
	}
	out := append([]string{}, body...)
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, foot...)
}

func (p *Panel) viewPicker() []string {
	w := p.w
	body := []string{
		bold + fit(" "+i18n.T("pick.project"), w) + reset, "",
		fit(" > "+p.filter+"█", w), "",
	}
	if len(p.filtered) == 0 {
		body = append(body, dim+fit(i18n.T("list.nothing"), w)+reset)
	}
	for i, pr := range p.filtered {
		mark := "  "
		if pr.Last > 0 {
			mark = "· "
		}
		line := fit(" "+mark+pr.Rel, w)
		if i == p.pickSel {
			line = bgSel + line + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints(tr("hint.type", "hint.pick", "hint.ok", "hint.cancel", "hint.used"), w)...)
	return layout(body, foot, p.h, p.pickSel+4)
}

func (p *Panel) viewProfiles() []string {
	w := p.w
	title := i18n.T("pick.profile")
	switch {
	case p.resume != nil:
		title = i18n.T("pick.resumeAccount")
	case p.worktreeOf != "":
		title = i18n.T("pick.worktreeProfile")
	case p.switching != "":
		title = i18n.T("pick.switchAccount")
	}
	body := []string{bold + fit(" "+title, w) + reset, dim + fit(" "+p.d.Paths.Pretty(p.chosenDir), w) + reset, ""}
	for i, pr := range p.profChoices {
		text := fmt.Sprintf(" %d  %-16s %s", i+1, pr.Name, pr.Group)
		if i == p.profSel {
			body = append(body, bgSel+fit(text, w)+reset)
		} else {
			body = append(body, fit(text, w))
		}
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints(tr("hint.pick", "hint.open", "hint.back"), w)...)
	return layout(body, foot, p.h, p.profSel+3)
}

func (p *Panel) viewHistory() []string {
	w := p.w
	body := []string{
		bold + fit(" "+i18n.T("history.title", len(p.histAll)), w) + reset, "",
		fit(" > "+p.filter+"█", w), "",
	}
	if len(p.histShown) == 0 {
		body = append(body, dim+fit(i18n.T("list.nothing"), w)+reset)
	}
	now := time.Now()
	projW := min(22, max(10, w/5))
	const ageW = 4
	tagW := 4
	titleW := w - 1 - projW - 2 - tagW - 1 - ageW - 1
	for i, e := range p.histShown {
		line := " " + fit(filepath.Base(e.Dir), projW) + "  " + fit(e.Title, titleW) + " " +
			fit(p.entryTag(e), tagW) + fmt.Sprintf("%*s ", ageW, status.Short(now.Sub(e.Updated)))
		switch {
		case i == p.histSel:
			line = bgSel + fit(line, w) + reset
		case e.Profile == "":
			line = dim + fit(line, w) + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints(tr("hint.typeHistory", "hint.pick", "hint.resume", "hint.back"), w)...)
	return layout(body, foot, p.h, p.histSel+4)
}

// tr translates a list of message keys.
func tr(keys ...string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = i18n.T(k)
	}
	return out
}

// helpKeys lists every key with its description, in screen order.
var helpKeys = []struct{ key, msg string }{
	{"↑ ↓  j k", "help.move"},
	{"Enter", "help.enter"},
	{"Ctrl+\\", "help.toggle"},
	{"Tab", "help.tab"},
	{"1-9", "help.number"},
	{"n", "help.new"},
	{"w", "help.worktree"},
	{"h", "help.history"},
	{"/", "help.filter"},
	{"r", "help.rename"},
	{"a", "help.account"},
	{"y", "help.answer"},
	{"Esc", "help.hide"},
	{"x", "help.close"},
	{"z", "help.zoom"},
	{"q", "help.quit"},
}

func (p *Panel) viewHelp() []string {
	w := p.w
	body := []string{bold + fit(" "+i18n.T("help.title"), w) + reset, ""}
	for _, h := range helpKeys {
		body = append(body, " "+bold+fit(h.key, 10)+reset+" "+fit(i18n.T(h.msg), w-12))
	}
	body = append(body, "", dim+fit(" "+i18n.T("help.status"), w)+reset)
	foot := []string{dim + strings.Repeat("─", w) + reset, dim + fit(" "+i18n.T("help.close.any"), w) + reset}
	return layout(body, foot, p.h, 0)
}

// statsSpans is the footer's machine line: CPU, memory and processes created
// per minute. Colors only show up when something is high; the CPU sparkline
// only when there was load in the last 16 s.
func (p *Panel) statsSpans() []span {
	r := p.reading
	var out []span
	calm := func(color string) string {
		if color == cDone {
			return ""
		}
		return color
	}
	if r.CPU >= 0 {
		out = append(out, sp(" CPU ", cQuiet))
		if peak(p.cpuHist) >= 25 {
			out = append(out, sp(spark(p.cpuHist)+" ", loadColor(r.CPU)))
		}
		out = append(out, sp(fmt.Sprintf("%.0f%%", r.CPU), calm(loadColor(r.CPU))))
	}
	if m := r.MemPercent(); m >= 0 {
		const cells = 6
		full := min(int(m/100*cells+0.5), cells)
		barColor := cQuiet
		if c := memColor(m); c != cDone {
			barColor = c
		}
		out = append(out, sp("   Mem ", cQuiet), sp(gb(r.MemUsed)+"/"+gb(r.MemTotal)+" GB ", calm(memColor(m))),
			sp(strings.Repeat("━", full), barColor), sp(strings.Repeat("━", cells-full), cFaint))
	}
	if r.PerMinute >= 0 {
		out = append(out, sp("   "+i18n.T("list.procs")+" ", cQuiet), sp(fmt.Sprintf("%.0f", r.PerMinute), ""))
	}
	return out
}

func peak(vals []float64) float64 {
	m := 0.0
	for _, v := range vals {
		m = max(m, v)
	}
	return m
}

// gb formats bytes as gigabytes with one decimal ("11.2"), or none from 100 up.
func gb(b uint64) string {
	v := float64(b) / (1 << 30)
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// promptLines shows, under a session waiting for you, what it asks: the
// context (the command) and the options, with "y" to answer from here.
func (p *Panel) promptLines(pr prompt, w int) []string {
	lead := []span{sp("▌", cAmber), sp("      ", "")}
	var out []string
	if text := pr.Context; text != "" || pr.Question != "" {
		if text == "" {
			text = pr.Question
		}
		out = append(out, line(w, bgWait, append(lead, sp(text, cQuiet))...))
	}
	var opts []string
	for i, o := range pr.Options {
		opts = append(opts, fmt.Sprintf("%d %s", i+1, short(o, 16)))
	}
	hint := i18n.T("prompt.hint")
	optW := max(w-7-1-width(hint)-2, 8)
	out = append(out, line(w, bgWait, append(lead,
		sp(fit(strings.Join(opts, " · "), optW), cWaiting), sp("  ", ""), sp(hint, cAccent))...))
	return out
}

// answerLines is the footer while answering a session (key y).
func (p *Panel) answerLines(w int) []string {
	pr := p.prompts[p.answering]
	name := ""
	for _, a := range p.agents {
		if a.ID == p.answering {
			name = p.sessionText(a)
		}
	}
	out := []string{line(w, "", sp(" "+i18n.T("prompt.title", name), yellow))}
	if pr.Context != "" {
		out = append(out, line(w, "", sp("   "+pr.Context, cQuiet)))
	}
	if pr.Question != "" {
		out = append(out, line(w, "", sp("   "+pr.Question, "")))
	}
	for i, o := range pr.Options {
		style := ""
		mark := "  "
		if i == pr.Selected {
			style, mark = cAccent, "❯ "
		}
		out = append(out, line(w, "", sp("   "+mark, cAccent), sp(fmt.Sprintf("%d  %s", i+1, o), style)))
	}
	return append(out, line(w, "", sp(" "+i18n.T("prompt.keys"), cFaint)))
}
