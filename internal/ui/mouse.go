package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ---------- width ----------

// Panel width limits, in columns.
const (
	minPanelW = 24
	stepW     = 4
)

// applyWidth gives the panel the width it was last dragged to.
func (p *Panel) applyWidth() {
	if w := p.prefs.PanelWidth; w >= minPanelW {
		_, _ = p.d.Tmux.Run("resize-pane", "-t", p.d.Self, "-x", strconv.Itoa(w))
	}
}

// noteWidth remembers a width the user chose (dragging the border, or < >),
// not the full width of the help screen's zoom.
func (p *Panel) noteWidth() {
	if p.w == p.prefs.PanelWidth || p.mode == modeHelp {
		return
	}
	if out, err := p.d.Tmux.Run("display", "-p", "-t", p.d.Self, "#{window_zoomed_flag}"); err != nil || out == "1" {
		return
	}
	p.widthDirty = true
}

// growPanel widens (n > 0) or narrows the list from the keyboard (> and <).
func (p *Panel) growPanel(n int) {
	dir := "-R"
	if n < 0 {
		dir, n = "-L", -n
	}
	if dir == "-L" && p.w-n < minPanelW {
		return
	}
	_, _ = p.d.Tmux.Run("resize-pane", "-t", p.d.Self, dir, strconv.Itoa(n))
}

// ---------- mouse ----------

// mouseKey turns an SGR mouse report ("<0;12;5" + M or m) into a key name:
// "mouse:click:<row>", "mouse:wheelup", "mouse:wheeldown"; "" to ignore.
func mouseKey(params string, final byte) string {
	f := strings.Split(strings.TrimPrefix(params, "<"), ";")
	if len(f) != 3 || final != 'M' { // only presses; releases are ignored
		return ""
	}
	b, err1 := strconv.Atoi(f[0])
	x, err3 := strconv.Atoi(f[1])
	y, err2 := strconv.Atoi(f[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	switch b &^ (4 | 8 | 16) { // without shift, meta, ctrl
	case 0:
		return fmt.Sprintf("mouse:click:%d:%d", y-1, x-1)
	case 64:
		return "mouse:wheelup"
	case 65:
		return "mouse:wheeldown"
	}
	return ""
}

// handleMouse: a click selects what is under it; a second click on the same
// line soon after acts like Enter. The wheel moves the selection.
func (p *Panel) handleMouse(k string) {
	switch k {
	case "mouse:wheelup":
		p.wheel("up")
		return
	case "mouse:wheeldown":
		p.wheel("down")
		return
	}
	pos := strings.Split(strings.TrimPrefix(k, "mouse:click:"), ":")
	row, err := strconv.Atoi(pos[0])
	if err != nil {
		return
	}
	col := -1
	if len(pos) > 1 {
		col, _ = strconv.Atoi(pos[1])
	}
	switch p.mode {
	case modeHelp, modeTimeline:
		p.closePicker()
		return
	case modeOverview:
		p.clickTile(row, col)
		return
	}
	if row < 0 || row >= len(p.rowTarget) || p.rowTarget[row] < 0 {
		return
	}
	target := p.rowTarget[row]
	now := time.Now()
	double := row == p.lastRow && now.Sub(p.lastClick) < 400*time.Millisecond
	p.lastRow, p.lastClick = row, now
	if double {
		p.lastClick = time.Time{} // a third click starts over
	}

	switch p.mode {
	case modeList, modeFilter:
		p.mode = modeList
		p.clickItem(target, double)
	case modePickDir:
		p.pickSel = target
		if double {
			p.keyPickDir("enter")
		}
	case modeHistory:
		p.histSel = target
		if double {
			p.keyHistory("enter")
		}
	case modePickProfile:
		p.profSel = target
		if double {
			p.keyPickProfile("enter")
		}
	case modeTemplates:
		p.tplSel = target
		if double {
			p.pickTemplate(target)
		}
	}
}

// clickItem: one click on a session shows it on the right (the focus stays
// in the list), two enter it. One click opens or closes a folder. A past
// conversation needs two clicks to resume, so a stray click starts nothing.
func (p *Panel) clickItem(i int, double bool) {
	if i >= len(p.items) {
		return
	}
	p.sel = i
	it := p.items[i]
	switch {
	case it.agent >= 0:
		p.swapC = nil
		if double {
			p.focusSession()
		} else {
			p.showSelected()
		}
	case it.folder:
		if !double { // the second click of a double would close it again
			p.toggleFolder(nil)
		}
	case double:
		p.resumeEntry(it.entry)
	}
}

func (p *Panel) wheel(dir string) {
	switch p.mode {
	case modeList:
		p.keyList(dir)
	case modePickDir:
		p.keyPickDir(dir)
	case modeHistory:
		p.keyHistory(dir)
	case modePickProfile:
		p.keyPickProfile(dir)
	case modeTemplates:
		p.keyTemplates(dir)
	}
}

// ---------- hidden sessions keep the right side's size ----------

// syncHiddenSize gives the windows of the hidden sessions the exact size of
// the right side. Otherwise a hidden session lives at the full terminal
// width, and showing it resizes it: Claude redraws and the content jumps.
// w, h is the right side's size; 0 when unknown (nothing changes).
func (p *Panel) syncHiddenSize(w, h int) {
	if w < 10 || h < 4 {
		return
	}
	size := fmt.Sprintf("%dx%d", w, h)
	var args []string
	for _, a := range p.agents {
		if a.Displayed || a.Pane == "" || p.sized[a.Pane] == size {
			continue
		}
		p.sized[a.Pane] = size
		if args != nil {
			args = append(args, ";")
		}
		args = append(args, "set-option", "-w", "-t", a.Pane, "window-size", "manual", ";",
			"resize-window", "-t", a.Pane, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h))
	}
	if args != nil {
		_, _ = p.d.Tmux.Run(args...)
	}
}

// displaySize asks tmux for the right side's size (after the list resized).
func (p *Panel) displaySize() (int, int) {
	if p.displayPane == "" {
		return 0, 0
	}
	out, err := p.d.Tmux.Run("display", "-p", "-t", p.displayPane, "#{pane_width} #{pane_height} #{window_zoomed_flag}")
	if err != nil {
		return 0, 0
	}
	var w, h, z int
	if n, _ := fmt.Sscan(out, &w, &h, &z); n != 3 || z == 1 {
		return 0, 0
	}
	return w, h
}
