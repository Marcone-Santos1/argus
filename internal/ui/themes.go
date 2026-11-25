package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/install"
)

// ---------- the theme window (key T) ----------
//
// The Argus theme's sets, each with a strip of its colors. Moving over one
// shows it in Argus at once; ⏎ applies it to Claude and the editor too, esc
// goes back to the one in use.

var themeModes = install.ThemeModes

// themeSwatch: background, card, Claude, opencode, done, waiting.
func themeSwatch(mode string) []string {
	if p, ok := install.Palettes[mode]; ok {
		return []string{p.Main, p.Line2, p.Claude, p.OC, p.Green, p.Amber}
	}
	if mode == install.ThemeLight {
		return []string{"#ffffff", "#e4e4e9", "#c4613f", "#4f5bd5", "#15803d", "#b45309"}
	}
	return []string{"#0f0f11", "#26262b", "#d97757", "#9aa6ff", "#4ade80", "#f5b14c"}
}

func (p *Panel) openThemes() {
	p.themeSel = 0
	for i, m := range themeModes {
		if m == p.argusMode() {
			p.themeSel = i
		}
	}
	p.mode = modeTheme
}

// previewTheme shows the chosen set everywhere it will go: Argus (the app
// look), the editor and, when Claude is on the Argus theme, the sessions.
// Nothing is saved until ⏎.
func (p *Panel) previewTheme() {
	mode := themeModes[p.themeSel]
	if p.appLook() && mode != paletteMode {
		setPalette(mode)
		p.repaint()
	}
	p.setFreshTheme(mode)
	if p.claudeTheme == argusTheme {
		p.writeArgusTheme(mode)
	}
}

func (p *Panel) keyTheme(k string) {
	switch k {
	case "esc", "ctrl-C", "q":
		p.closePicker()
		if p.claudeTheme == argusTheme { // back to the set in use
			p.writeArgusTheme(p.argusMode())
		}
		p.followTheme()
	case "up", "k":
		if p.themeSel > 0 {
			p.themeSel--
			p.previewTheme()
		}
	case "down", "j":
		if p.themeSel < len(themeModes)-1 {
			p.themeSel++
			p.previewTheme()
		}
	case "enter":
		p.closePicker()
		p.setArgusTheme(themeModes[p.themeSel])
	default:
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(themeModes) {
			p.themeSel = n - 1
			p.keyTheme("enter")
		}
	}
}

// hexBg is a background escape for "#rrggbb".
func hexBg(h string) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	if err != nil {
		return ""
	}
	return bgc(int(v>>16), int(v>>8&0xff), int(v&0xff))
}

func (p *Panel) viewThemes() []string {
	w := p.w
	body := []string{line(w, "", sp(" "+i18n.T("theme.title"), "\x1b[1m")), ""}
	first := len(body)
	inUse := ""
	if p.claudeTheme == argusTheme || p.claudeTheme == "" {
		inUse = p.argusMode()
	}
	for i, m := range themeModes {
		bg, bar := rowBar(i == p.themeSel, false)
		name := i18n.T("theme." + m)
		row := []span{bar, sp(fmt.Sprintf(" %d  ", i+1), cQuiet), sp(fmt.Sprintf("%-10s", name), "\x1b[1m"), sp(" ", "")}
		for _, c := range themeSwatch(m) {
			row = append(row, sp("  ", hexBg(c)))
		}
		if m == inUse {
			row = append(row, sp("  ✓ "+i18n.T("theme.now"), cDone))
		}
		body = append(body, line(w, bg, row...))
		body = append(body, line(w, bg, sp("      "+i18n.T("theme."+m+".d"), cFaint)))
	}
	if inUse == "" {
		body = append(body, "", line(w, "", sp(" "+i18n.T("theme.other"), cQuiet)))
	}
	foot := []string{line(w, "", sp(strings.Repeat("─", w), cFaint))}
	for _, h := range wrapHints(tr("hint.pick", "hint.apply", "hint.back"), w) {
		foot = append(foot, line(w, "", sp(stripANSI(h), cFaint)))
	}
	// Two lines per set: a click on either picks it.
	targets := make([]int, first, first+2*len(themeModes))
	for i := range targets {
		targets[i] = -1
	}
	for i := range themeModes {
		targets = append(targets, i, i)
	}
	return p.layoutClick(body, targets, foot, first+2*p.themeSel)
}
