package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/Marcone-Santos1/argus/internal/history"
)

// ---------- each session's background follows its Claude theme ----------
//
// A light Claude theme draws dark text for a light terminal. tmux paints
// that session's pane white, so the theme looks as Claude means it whatever
// the terminal's own background; a dark theme gets the look's background.

// themeOf is the theme in a Claude profile's settings ("" when unknown),
// re-read only when the file changes.
func (p *Panel) themeOf(profile string) string {
	prof, ok := p.profileOf(profile)
	if !ok || prof.Label() != history.Claude {
		return ""
	}
	path := filepath.Join(prof.ClaudeConfigDir(p.d.Paths.Home), "settings.json")
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if c, ok := p.themes[path]; ok && c.at.Equal(fi.ModTime()) {
		return c.theme
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	t := gjson.GetBytes(data, "theme").String()
	p.themes[path] = themeCache{fi.ModTime(), t}
	return t
}

type themeCache struct {
	at    time.Time
	theme string
}

// syncPaneBg gives a session's pane the background its theme expects.
func (p *Panel) syncPaneBg(i int) {
	a := p.agents[i]
	want := ""
	switch {
	case strings.HasPrefix(p.themeOf(a.Profile), "light"):
		want = "bg=#ffffff"
	case p.appLook():
		want = "bg=" + aMain
	}
	if p.paneBg[a.Pane] == want || a.Pane == "" {
		return
	}
	var err error
	if want == "" {
		_, err = p.d.Tmux.Run("set", "-pu", "-t", a.Pane, "window-style", ";", "set", "-pu", "-t", a.Pane, "window-active-style")
	} else {
		err = p.d.Tmux.SetPaneOptions(a.Pane, "window-style", want, "window-active-style", want)
	}
	if err == nil {
		p.paneBg[a.Pane] = want
	}
}

// themeCheck re-reads the themes every 10 s (a stat per Claude account),
// so a theme picked with /theme reaches the pane without a restart.
func (p *Panel) themeCheck() {
	if p.themeTick++; p.themeTick%5 != 0 {
		return
	}
	for i := range p.agents {
		p.syncPaneBg(i)
	}
}
