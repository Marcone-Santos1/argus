package ui

import (
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

// ---------- overview (key v): every session's screen side by side ----------

// ovLayout is where the tiles were drawn, to map a click to a tile.
type ovLayout struct {
	cols, tileW, tileH, top, gap int
	order                        []int // tile -> index in p.agents
}

// ovOrder is the sessions in list order (grouped by folder).
func (p *Panel) ovOrder() []int {
	var out []int
	for _, it := range p.items {
		if it.agent >= 0 {
			out = append(out, it.agent)
		}
	}
	return out
}

func (p *Panel) openOverview() {
	if len(p.agents) == 0 {
		return
	}
	p.mode = modeOverview
	p.ovSel = 0
	if a := p.current(); a != nil {
		for i, idx := range p.ovOrder() {
			if p.agents[idx].ID == a.ID {
				p.ovSel = i
			}
		}
	}
	p.setZoom(true)
	p.captureScreens()
}

// captureScreens reads, off the main loop, what each session shows. It runs
// only while the overview is open (on opening and on the 2 s tick).
func (p *Panel) captureScreens() {
	if p.capturing {
		return
	}
	p.capturing = true
	type shot struct{ id, pane string }
	var shots []shot
	for _, a := range p.agents {
		shots = append(shots, shot{a.ID, a.Pane})
	}
	tm, ch := p.d.Tmux, p.asyncC
	go func() {
		got := map[string][]string{}
		for _, s := range shots {
			if out, err := tm.Run("capture-pane", "-p", "-t", s.pane); err == nil {
				got[s.id] = strings.Split(out, "\n")
			}
		}
		ch <- func() {
			p.capturing = false
			p.screens = got
		}
	}()
}

// screenTail is the end of a session's screen without Claude's input box and
// what is below it (status line, agent list): the box starts at a rule right
// above a "❯" line. Runs of blank lines become one.
func screenTail(lines []string, n int) []string {
	for i := len(lines) - 1; i > 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "❯") && isBoxTop(lines[i-1]) {
			lines = lines[:i-1]
			break
		}
	}
	var out []string
	for _, l := range lines {
		l = strings.TrimRight(l, " ")
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// isBoxTop is the rule that opens Claude's input box; it may carry a title
// at its end ("──── session name").
func isBoxTop(l string) bool {
	return strings.HasPrefix(strings.TrimSpace(l), strings.Repeat("─", 10))
}

func (p *Panel) viewOverview() []string {
	w, h := p.w, p.h
	order := p.ovOrder()
	foot := []string{line(w, "", sp(strings.Repeat("─", w), cFaint))}
	for _, l := range wrapHints(tr("hint.ovMove", "hint.open", "hint.back"), w) {
		foot = append(foot, line(w, "", sp(stripANSI(l), cFaint)))
	}
	if len(order) == 0 {
		return layout([]string{line(w, "", sp(i18n.T("list.empty1"), cQuiet))}, foot, h, 0)
	}
	const gap = 2
	cols := min(max(w/60, 1), len(order))
	rows := (len(order) + cols - 1) / cols
	tileW := (w - gap*(cols-1)) / cols
	room := h - len(foot)
	tileH := max((room-(rows-1))/rows, 4)
	p.ov = ovLayout{cols: cols, tileW: tileW, tileH: tileH, gap: gap, order: order}
	p.ovSel = clamp(p.ovSel, len(order))

	now := time.Now()
	var body []string
	focus := 0
	for r := 0; r < rows; r++ {
		if r > 0 {
			body = append(body, "")
		}
		if p.ovSel/cols == r {
			focus = len(body)
		}
		tiles := make([][]string, cols)
		for c := 0; c < cols; c++ {
			i := r*cols + c
			if i >= len(order) {
				tiles[c] = nil
				continue
			}
			tiles[c] = p.tile(p.agents[order[i]], i == p.ovSel, tileW, tileH, now)
		}
		for y := 0; y < tileH; y++ {
			var b strings.Builder
			for c := 0; c < cols; c++ {
				if c > 0 {
					b.WriteString(strings.Repeat(" ", gap))
				}
				if y < len(tiles[c]) {
					b.WriteString(tiles[c][y])
				} else {
					b.WriteString(strings.Repeat(" ", tileW))
				}
			}
			body = append(body, b.String())
		}
	}
	out, start := layoutAt(body, foot, h, focus)
	p.ov.top = -start
	return out
}

// tile is one session in the overview: a header (state, title, account,
// model, time) and the end of its screen.
func (p *Panel) tile(a agent, sel bool, w, h int, now time.Time) []string {
	icon, color, _ := statusStyle(a.Status)
	if a.Status == status.Working || p.shells[a.ID] > 0 {
		icon, color = spinFrames[int(now.UnixMilli()/500)%len(spinFrames)], cWorking
	}
	bg := bgSoft
	switch {
	case sel:
		bg = bgSel
	case a.Status == status.Waiting:
		bg = bgWait
	}
	info := ""
	if in, ok := p.usage[a.ID]; ok && in.Model != "" {
		info = in.Model + " · "
	}
	if !a.Since.IsZero() {
		info += status.Short(now.Sub(a.Since))
	}
	title := lineLR(w, bg,
		[]span{sp(" "+icon+" ", color), sp(p.sessionText(a), "\x1b[1m"), sp("  "+p.tag(a.Profile), p.accountColor(a.Profile))},
		[]span{sp(info, cQuiet), sp(" ", "")})
	out := []string{title}
	style := cQuiet
	if a.Status == status.Waiting {
		style = ""
	}
	for _, l := range screenTail(p.screens[a.ID], h-1) {
		out = append(out, line(w, "", sp(" "+l, style)))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

func (p *Panel) keyOverview(k string) {
	n := len(p.ov.order)
	cols := max(p.ov.cols, 1)
	switch k {
	case "esc", "ctrl-C", "q", "v":
		p.closePicker()
	case "left", "h":
		p.ovSel = max(p.ovSel-1, 0)
	case "right", "l", "tab":
		p.ovSel = min(p.ovSel+1, n-1)
	case "up", "k":
		if p.ovSel-cols >= 0 {
			p.ovSel -= cols
		}
	case "down", "j":
		if p.ovSel+cols < n {
			p.ovSel += cols
		}
	case "enter":
		p.openTile(p.ovSel)
	default:
		if len(k) == 1 && k >= "1" && k <= "9" {
			p.openTile(int(k[0] - '1'))
		}
	}
}

// openTile leaves the overview and enters that session.
func (p *Panel) openTile(i int) {
	if i < 0 || i >= len(p.ov.order) {
		return
	}
	id := p.agents[p.ov.order[i]].ID
	p.closePicker()
	if p.selectKey("a:" + id) {
		p.focusSession()
	}
}

// clickTile: one click selects a tile, a second one opens it.
func (p *Panel) clickTile(row, col int) {
	l := p.ov
	if col < 0 || l.tileW <= 0 {
		return
	}
	y := row - l.top
	r, c := y/(l.tileH+1), col/(l.tileW+l.gap)
	if y < 0 || y%(l.tileH+1) == l.tileH || c >= l.cols {
		return
	}
	i := r*l.cols + c
	if i >= len(l.order) {
		return
	}
	now := time.Now()
	double := i == p.ovSel && now.Sub(p.lastClick) < 400*time.Millisecond
	p.ovSel, p.lastClick = i, now
	if double {
		p.lastClick = time.Time{}
		p.openTile(i)
	}
}
