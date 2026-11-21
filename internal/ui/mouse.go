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
	y, err2 := strconv.Atoi(f[2])
	if err1 != nil || err2 != nil {
		return ""
	}
	switch b &^ (4 | 8 | 16) { // without shift, meta, ctrl
	case 0:
		return fmt.Sprintf("mouse:click:%d", y-1)
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
	row, err := strconv.Atoi(strings.TrimPrefix(k, "mouse:click:"))
	if err != nil {
		return
	}
	if p.mode == modeHelp {
		p.closePicker()
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
	}
}
