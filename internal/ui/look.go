package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// ---------- the "app" look (key L): neutral colors, cards, sections ----------
//
// An alternative drawing of the list, kept beside the classic one while it is
// tried out. Only the list changes; keys, items and actions are the same.

const (
	lookApp     = "app"
	lookClassic = "classic"
)

// Pseudo folders of the app look: the hibernated sessions and the past
// conversations, each collapsed into one row until opened.
const (
	asleepKey = "\x00asleep"
	recentKey = "\x00recent"
)

// Colors of the app look.
const (
	aBg     = "#141417" // the panel's background
	aMain   = "#0f0f11" // the sessions' and the editor's background
	aCard   = "\x1b[48;2;38;38;43m"
	aNeed   = "\x1b[48;2;44;34;22m"
	aSoft   = "\x1b[38;2;180;180;188m"
	aMuted  = "\x1b[38;2;125;125;135m"
	aFaint  = "\x1b[38;2;80;80;90m"
	aGreen  = "\x1b[38;2;74;222;128m"
	aAmber  = "\x1b[38;2;245;177;76m"
	aRed    = "\x1b[38;2;248;113;113m"
	aClaude = "\x1b[38;2;217;119;87m"
	aOC     = "\x1b[38;2;154;166;255m"
	aBlue   = "\x1b[38;2;124;179;255m"
	pillRun = "\x1b[48;2;29;51;37m\x1b[38;2;74;222;128m"
	pillNd  = "\x1b[48;2;74;53;23m\x1b[38;2;245;177;76m"
	pillN   = "\x1b[48;2;43;43;48m\x1b[38;2;180;180;188m"
)

// appLook: the app look is the default; L switches to the classic one.
func (p *Panel) appLook() bool { return p.prefs.Look != lookClassic }

// toggleLook switches between the classic and the app look and remembers it.
func (p *Panel) toggleLook() {
	if p.appLook() {
		p.prefs.Look = lookClassic
	} else {
		p.prefs.Look = lookApp
	}
	_ = p.prefs.Save()
	p.applyLook()
	p.prevLines = nil // full redraw: the background changed
	p.rebuildItems(p.selKey())
	p.flash(i18n.T("msg.look." + map[bool]string{true: "app", false: "classic"}[p.appLook()]))
}

// applyLook sets what tmux draws for the look: the panel's background, the
// borders and, in the app look, the status line at the bottom.
func (p *Panel) applyLook() {
	if p.d.Self == "" {
		return
	}
	s := tmux.Session
	if p.appLook() {
		_, _ = p.d.Tmux.Run("set", "-p", "-t", p.d.Self, "window-style", "bg="+aBg,
			";", "set", "-p", "-t", p.d.Self, "window-active-style", "bg="+aBg,
			";", "set", "-g", "window-style", "bg="+aMain,
			";", "set", "-g", "window-active-style", "bg="+aMain,
			";", "set", "-g", "pane-border-style", "fg=#26262b,bg="+aMain,
			";", "set", "-g", "pane-active-border-style", "fg=#33333a,bg="+aMain,
			";", "set", "-t", s, "status-style", "bg="+aBg+",fg=#7d7d87",
			";", "set", "-t", s, "status-format[0]", "#{@argus_tabs}",
			";", "set", "-t", s, "status-position", "top",
			";", "set", "-t", s, "status", "on")
		p.tabsText = ""
	} else {
		_, _ = p.d.Tmux.Run("set", "-pu", "-t", p.d.Self, "window-style",
			";", "set", "-pu", "-t", p.d.Self, "window-active-style",
			";", "set", "-gu", "window-style",
			";", "set", "-gu", "window-active-style",
			";", "set", "-g", "pane-border-style", "fg=#3b4048",
			";", "set", "-g", "pane-active-border-style", "fg=#4d6491",
			";", "set", "-u", "-t", s, "status-style",
			";", "set", "-u", "-t", s, "status-format",
			";", "set", "-u", "-t", s, "status-position",
			";", "set", "-u", "-t", s, "status")
	}
}

// tmuxSpans writes spans in tmux's style syntax (#[fg=...]), for the status
// line and the tabs.
func tmuxSpans(spans []span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString("#[default")
		if fg := escHex(s.style, "38;2;"); fg != "" {
			b.WriteString(",fg=" + fg)
		}
		if bg := escHex(s.style, "48;2;"); bg != "" {
			b.WriteString(",bg=" + bg)
		}
		if strings.Contains(s.style, "\x1b[1") {
			b.WriteString(",bold")
		}
		b.WriteString("]" + strings.ReplaceAll(s.text, "#", "##"))
	}
	return b.String()
}

// escHex finds a 24-bit color ("38;2;r;g;b") in an escape and returns it as #rrggbb.
func escHex(esc, prefix string) string {
	i := strings.Index(esc, prefix)
	if i < 0 {
		return ""
	}
	var r, g, b int
	if n, _ := fmt.Sscanf(esc[i+len(prefix):], "%d;%d;%dm", &r, &g, &b); n != 3 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// tabsLine is, in the app look, tmux's status line at the top: one tab per
// open session, under the session on the right, the shown one highlighted.
// Each tab is a tmux range, so a click on it reaches the panel (see _tab).
func (p *Panel) tabsLine(shown string) string {
	type tab struct {
		text string
		w    int
	}
	var tabs []tab
	at := 0
	now := time.Now()
	for _, it := range p.items {
		if it.agent < 0 {
			continue
		}
		a := p.agents[it.agent]
		if !a.Asleep.IsZero() {
			continue
		}
		name := strings.TrimRight(fit(p.sessionText(a), 18), " ")
		style := "#[default,fg=#7d7d87]"
		if a.ID == shown {
			style = "#[default,bg=#26262b,fg=#ececf0,bold]"
			at = len(tabs)
		}
		t := style + " " + tmuxSpans([]span{p.appDot(a, now)}) + style + " " + tmuxSpans([]span{p.agentGlyph(a.Profile)}) +
			style + " " + strings.ReplaceAll(name, "#", "##") + " "
		tabs = append(tabs, tab{"#[range=user|" + a.ID + "]" + t + "#[norange]", width(stripTmux(t)) + 1})
	}
	// Keep the shown tab and its neighbours when they do not all fit.
	room := max(p.dispW-2, 20)
	total := 0
	for _, t := range tabs {
		total += t.w
	}
	lo, hi := 0, len(tabs)
	for total > room && hi-lo > 1 {
		if at-lo > hi-1-at {
			total -= tabs[lo].w
			lo++
		} else {
			total -= tabs[hi-1].w
			hi--
		}
	}
	var b strings.Builder
	pad := p.w + 1 // above the list (and the code column): empty
	if p.editorPane != "" {
		pad += p.edW + 1
	}
	b.WriteString("#[default]" + strings.Repeat(" ", pad))
	if lo > 0 {
		b.WriteString("#[default,fg=#4c4c55]‹")
	}
	for _, t := range tabs[lo:hi] {
		b.WriteString(t.text + "#[default] ")
	}
	if hi < len(tabs) {
		b.WriteString("#[default,fg=#4c4c55]›")
	}
	return b.String() + "#[default]"
}

// syncTabs writes the tabs row when it changes.
func (p *Panel) syncTabs() {
	shown := ""
	for _, a := range p.agents {
		if a.Displayed {
			shown = a.ID
		}
	}
	t := p.tabsLine(shown)
	if t == p.tabsText {
		return
	}
	if _, err := p.d.Tmux.Run("set", "-t", tmux.Session, "@argus_tabs", t); err == nil {
		p.tabsText = t
	}
}

// clickTab shows the session whose tab was clicked.
func (p *Panel) clickTab(id string) {
	if p.appLook() && p.selectKey("a:"+id) {
		p.swapC = nil
		p.showSelected()
	}
}

// stripTmux removes tmux style sequences (#[...]) and unescapes ##.
func stripTmux(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i+1 < len(s) {
			if s[i+1] == '[' {
				if j := strings.IndexByte(s[i:], ']'); j >= 0 {
					i += j
					continue
				}
			}
			if s[i+1] == '#' {
				b.WriteByte('#')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// appItems reorders the sessions for the app look: those waiting for you
// first, then the rest by folder, then the hibernated ones (one row while
// collapsed). The past conversations become one row until opened.
func (p *Panel) appItems(sessions, past []item) []item {
	var need, rest, asleep []item
	for _, it := range sessions {
		a := p.agents[it.agent]
		switch {
		case !a.Asleep.IsZero():
			asleep = append(asleep, it)
		case a.Status == status.Waiting:
			need = append(need, it)
		default:
			rest = append(rest, it)
		}
	}
	out := need
	// Each project gets a heading; a collapsed one hides its sessions.
	for i := 0; i < len(rest); {
		d := rest[i].dir
		j := i
		for j < len(rest) && rest[j].dir == d {
			j++
		}
		out = append(out, item{dir: d, agent: -1, folder: true, header: true, count: j - i})
		if !p.prefs.Collapsed[d] || p.listFilter != "" {
			out = append(out, rest[i:j]...)
		}
		i = j
	}
	if len(asleep) > 0 {
		if p.expanded[asleepKey] || p.listFilter != "" {
			out = append(out, asleep...)
		} else {
			out = append(out, item{dir: asleepKey, agent: -1, folder: true, count: len(asleep)})
		}
	}
	if len(past) > 0 {
		if p.expanded[recentKey] || p.listFilter != "" {
			out = append(out, past...)
		} else {
			n := 0
			for _, it := range past {
				if it.folder {
					n++
				}
			}
			out = append(out, item{dir: recentKey, agent: -1, folder: true, count: n})
		}
	}
	return out
}

// agentGlyph is the agent's mark: Claude's orange asterisk, opencode's
// diamond, Gemini's star.
func (p *Panel) agentGlyph(profile string) span {
	pr, _ := p.profileOf(profile)
	switch pr.Label() {
	case "opencode":
		return sp("◆", aOC)
	case "gemini":
		return sp("✦", aBlue)
	case "codex":
		return sp("◇", aSoft)
	}
	return sp("✳", aClaude)
}

// appDot is the session's state: a spinner while it works.
func (p *Panel) appDot(a agent, now time.Time) span {
	switch {
	case !a.Asleep.IsZero():
		return sp("◌", aFaint)
	case a.Status == status.Working || p.shells[a.ID] > 0:
		return sp(spinFrames[int(now.UnixMilli()/500)%len(spinFrames)], aAmber)
	case a.Status == status.Waiting:
		return sp("◐", aAmber)
	case a.Status == status.Done:
		return sp("●", aGreen)
	case a.Status == status.Ended:
		return sp("✕", aRed)
	}
	return sp("○", aMuted)
}

// card is a line inside a block with its own background, one column in from
// each edge.
func card(w int, bg string, left, right []span) string {
	return " " + lineLR(w-2, bg, append([]span{sp(" ", "")}, left...), append(right, sp(" ", ""))) + " "
}

// selCard is a line of the selected session's card: an orange bar on its
// left edge.
func selCard(w int, left, right []span) string {
	return " " + lineLR(w-2, aCard, append([]span{sp("▌", aClaude)}, left...), append(right, sp(" ", ""))) + " "
}

// appSection is a section's small heading with, on the right, a count pill
// or a key.
func appSection(w int, label string, right []span) string {
	return lineLR(w, "", []span{sp(" "+strings.ToUpper(label), aMuted)}, append(right, sp(" ", "")))
}

func (p *Panel) viewListApp() []string {
	w := p.w
	now := time.Now()
	var body []string
	var targets []int
	mark := func(t int) {
		for len(targets) < len(body) {
			targets = append(targets, t)
		}
	}

	// Search field: the / filter, always visible.
	field := []span{sp("⌕ ", aMuted), sp(i18n.T("app.search"), aFaint)}
	if p.mode == modeFilter || p.listFilter != "" {
		cursor := ""
		if p.mode == modeFilter {
			cursor = "█"
		}
		field = []span{sp("⌕ ", aSoft), sp(p.listFilter+cursor, "")}
	}
	body = append(body, "", card(w, aCard, field, []span{sp("/", aFaint)}), "")
	mark(-1)

	if len(p.agents) == 0 && p.listFilter == "" {
		body = append(body, line(w, "", sp(" "+i18n.T("list.empty1"), aMuted)), line(w, "", sp(" "+i18n.T("list.empty2"), aMuted)), "")
		mark(-1)
	}

	focus := 0
	sect, group := "", ""
	need := 0
	for _, it := range p.items {
		if it.agent >= 0 && p.agents[it.agent].Status == status.Waiting && p.agents[it.agent].Asleep.IsZero() {
			need++
		}
	}
	for idx, it := range p.items {
		sel := idx == p.sel && p.mode != modeFilter
		// Which section the item belongs to.
		s := "projects"
		var a agent
		if it.agent >= 0 {
			a = p.agents[it.agent]
			switch {
			case !a.Asleep.IsZero():
				s = "asleep"
			case a.Status == status.Waiting:
				s = "need"
			}
		} else if it.header {
			s = "projects"
		} else if it.dir == asleepKey {
			s = "asleep"
		} else {
			s = "recent"
		}
		if s != sect {
			if sect != "" {
				body = append(body, "")
			}
			switch s {
			case "need":
				body = append(body, appSection(w, i18n.T("app.need"), []span{sp(fmt.Sprintf(" %d ", need), pillNd)}))
			case "projects":
				body = append(body, appSection(w, i18n.T("app.projects"), []span{sp(i18n.T("hint.new"), aFaint)}))
			case "asleep":
				n := it.count
				if it.agent >= 0 {
					n = 0
					for _, o := range p.agents {
						if !o.Asleep.IsZero() {
							n++
						}
					}
				}
				body = append(body, appSection(w, i18n.T("app.asleep"), []span{sp(fmt.Sprintf(" %d ", n), pillN)}))
			case "recent":
				body = append(body, appSection(w, i18n.T("sec.recent"), []span{sp("h", aFaint)}))
			}
			mark(-1)
			sect, group = s, ""
		}
		_ = group
		if idx == p.sel {
			focus = len(body)
		}
		switch {
		case it.agent >= 0 && s == "need":
			body = append(body, p.needCard(a, sel, now, w)...)
		case it.agent >= 0:
			body = append(body, p.appRows(a, sel, s == "asleep", now, w)...)
		case it.header:
			body = append(body, p.projectHeader(it, sel, now, w))
		case it.dir == asleepKey:
			var names []string
			for _, o := range p.agents {
				if !o.Asleep.IsZero() {
					names = append(names, p.sessionText(o))
				}
			}
			body = append(body, p.appFolder(sel, w, []span{sp("◌ ", aFaint), sp(strings.Join(names, " · "), aFaint)}))
		case it.dir == recentKey:
			body = append(body, p.appFolder(sel, w, []span{sp(i18n.T("app.folders", it.count), aMuted)}))
		case it.folder:
			body = append(body, p.folderRow(it, sel, now, w))
		default:
			body = append(body, p.entryRow(it.entry, sel, now, w))
		}
		mark(idx)
	}
	if p.hiddenFolders > 0 && p.listFilter == "" && p.expanded[recentKey] {
		body = append(body, line(w, "", sp(i18n.T("list.more", p.hiddenFolders), aFaint)))
	}
	if len(p.items) == 0 && p.listFilter != "" {
		body = append(body, line(w, "", sp(" "+i18n.T("list.nothing"), aMuted)))
	}
	mark(-1)
	p.syncPanelTitle()
	p.syncTabs()
	return p.layoutClick(body, targets, p.footer(), focus)
}

// appFolder is a collapsed group's row (hibernated or past conversations).
func (p *Panel) appFolder(sel bool, w int, left []span) string {
	if sel {
		return card(w, aCard, left, []span{sp("⏎", aMuted)})
	}
	return lineLR(w, "", append([]span{sp("  ", "")}, left...), []span{sp("▸ ", aFaint)})
}

// appRows is a session in a folder: one line, or a card with its details
// when selected.
func (p *Panel) appRows(a agent, sel, asleep bool, now time.Time, w int) []string {
	el := ""
	if !a.Since.IsZero() {
		el = status.Short(now.Sub(a.Since))
	}
	tag := sp(p.tag(a.Profile), p.accountColor(a.Profile))
	right := []span{tag, sp(" ", ""), sp(fmt.Sprintf("%3s", el), aFaint)}
	if !asleep && (a.Status == status.Working || p.shells[a.ID] > 0) {
		right = append([]span{sp(" "+i18n.T("st.working")+" ", pillRun), sp(" ", "")}, right...)
	}
	title := p.sessionText(a)
	if !sel {
		style := aSoft
		if asleep {
			style = aFaint
		}
		if a.Displayed {
			style = "\x1b[1m"
		}
		return []string{lineLR(w, "", []span{sp("   ", ""), p.appDot(a, now), sp(" ", ""), p.agentGlyph(a.Profile), sp(" "+title, style)}, append(right, sp(" ", "")))}
	}
	out := []string{selCard(w, []span{sp(" ", ""), p.appDot(a, now), sp(" ", ""), p.agentGlyph(a.Profile), sp(" "+title, "\x1b[1m")}, right)}
	indent := sp("      ", "")
	if r := p.reply[a.ID]; r != "" && (asleep || replyShown(a)) {
		out = append(out, selCard(w, []span{indent, sp(r, "\x1b[3m"+aSoft)}, nil))
	}
	var d []span
	add := func(s ...span) {
		if len(d) > 0 {
			d = append(d, sp(" · ", aFaint))
		}
		d = append(d, s...)
	}
	if asleep {
		add(sp(i18n.T("app.resume"), aMuted))
	}
	if in, ok := p.usage[a.ID]; ok {
		if in.Model != "" {
			add(sp(in.Model, aMuted))
		}
		if in.Context > 0 && !asleep {
			on, off := cells(in.Context, 4)
			c := ctxColor(in.Context)
			if c == cQuiet {
				c = aMuted
			}
			add(sp("ctx ", aFaint), sp(on, c), sp(off, aFaint), sp(fmt.Sprintf(" %.0f%%", in.Context), c))
		}
	}
	if r, ok := p.res[a.ID]; ok && r.RSS > 0 {
		add(sp(size(r.RSS), aMuted))
	}
	if bg := p.backgroundSpans(a); bg != nil {
		add(bg...)
	}
	if a.Worktree != "" {
		add(sp("⑂ worktree", aMuted))
	}
	if len(d) > 0 {
		out = append(out, selCard(w, append([]span{indent}, d...), p.cpuSpans(a.ID)))
	}
	return out
}

// needCard is a session waiting for you: what it asks, on an amber card.
func (p *Panel) needCard(a agent, sel bool, now time.Time, w int) []string {
	el := ""
	if !a.Since.IsZero() {
		el = status.Short(now.Sub(a.Since))
	}
	bg := aNeed
	bar := sp(" ", "")
	if sel {
		bar = sp("▌", aAmber)
	}
	right := []span{sp(p.tag(a.Profile), p.accountColor(a.Profile)), sp(" ", ""), sp(el, aAmber)}
	out := []string{card(w, bg, []span{bar, sp("◐ ", aAmber), p.agentGlyph(a.Profile), sp(" "+p.sessionText(a), "\x1b[1m")}, right)}
	q := ""
	if pr, ok := p.prompts[a.ID]; ok {
		q = pr.Context
		if q == "" {
			q = pr.Question
		}
	}
	if q == "" {
		q = i18n.T("st.waiting")
	}
	out = append(out, card(w, bg, []span{bar, sp("     ", ""), sp(q, aSoft)}, []span{sp(i18n.T("hint.answer"), aAmber)}))
	return out
}

// appPanelTitle is the panel's border title in the app look: the name and how
// many sessions need you, work and are done, in the look's colors.
func (p *Panel) appPanelTitle() string {
	var need, work, done int
	for _, a := range p.agents {
		switch {
		case !a.Asleep.IsZero():
		case a.Status == status.Waiting:
			need++
		case a.Status == status.Working || p.shells[a.ID] > 0:
			work++
		case a.Status == status.Done:
			done++
		}
	}
	t := " #[fg=#ececf0,bold]argus#[nobold]"
	if need > 0 {
		t += fmt.Sprintf("  #[fg=#f5b14c]◐ %d", need)
	}
	if work > 0 {
		t += fmt.Sprintf("  #[fg=#f5b14c]◜ %d", work)
	}
	if done > 0 {
		t += fmt.Sprintf("  #[fg=#4ade80]● %d", done)
	}
	return t + " #[default]"
}

// projectHeader is a project's heading: ▾ open, ▸ collapsed (with its
// sessions' states in a row), and the git state on the right.
func (p *Panel) projectHeader(it item, sel bool, now time.Time, w int) string {
	collapsed := p.prefs.Collapsed[it.dir] && p.listFilter == ""
	left := []span{sp(" ", ""), sp("▾ ", aFaint), sp(p.folderName(it.dir), "\x1b[1m")}
	if collapsed {
		left[1] = sp("▸ ", aMuted)
		left = append(left, sp("  ", ""))
		for _, a := range p.agents {
			if a.Dir == it.dir && a.Asleep.IsZero() && a.Status != status.Waiting {
				left = append(left, p.appDot(a, now))
			}
		}
	}
	right := []span{sp(p.gitText(it.dir), aFaint), sp(" ", "")}
	if sel {
		return selCard(w, left[1:], right)
	}
	return lineLR(w, "", left, right)
}
