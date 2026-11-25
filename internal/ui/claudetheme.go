package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/install"
)

// ---------- the look follows the theme picked in Claude Code ----------
//
// Each Claude account keeps its theme in its settings.json. The last one
// picked (with /theme, in any account) wins: it is copied to the other
// accounts, and the app look turns light or dark with it, list, tabs,
// sessions and editor alike. Checked every 10 s: a stat per account.

// themeFile is one account's settings, as last seen.
type themeFile struct {
	at    time.Time
	theme string
}

// claudeSettings are the settings files of the Claude accounts, one per
// real file (accounts may share one through a link).
func (p *Panel) claudeSettings() []string {
	var out []string
	seen := map[string]bool{}
	for _, pr := range p.d.Config.Profiles {
		if pr.Label() != history.Claude {
			continue
		}
		f := filepath.Join(pr.ClaudeConfigDir(p.d.Paths.Home), "settings.json")
		real, err := filepath.EvalSymlinks(f)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		out = append(out, real)
	}
	return out
}

// themeCheck looks for a newly picked theme and follows it.
func (p *Panel) themeCheck(force bool) {
	if !force {
		if p.themeTick++; p.themeTick%5 != 0 {
			return
		}
	}
	files := p.claudeSettings()
	newest, newestAt := "", time.Time{}
	changed := false
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		last, known := p.themeFiles[f]
		if known && last.at.Equal(fi.ModTime()) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		t := gjson.GetBytes(data, "theme").String()
		p.themeFiles[f] = themeFile{fi.ModTime(), t}
		if t != "" && (!known || t != last.theme) && fi.ModTime().After(newestAt) {
			newest, newestAt, changed = t, fi.ModTime(), true
		}
	}
	if !changed {
		return
	}
	// The two fixed themes of an earlier version became one: the set moves
	// to the mode.
	switch newest {
	case "custom:argus-dark", "custom:argus-light":
		p.prefs.ThemeMode = map[bool]string{true: install.ThemeLight, false: ""}[newest == "custom:argus-light"]
		_ = p.prefs.Save()
		newest = argusTheme
	}
	p.writeArgusTheme(p.argusMode())
	p.claudeTheme = newest
	// The last pick wins: the other accounts get it too.
	for _, f := range files {
		if p.themeFiles[f].theme != newest {
			if setTheme(f, newest) {
				if fi, err := os.Stat(f); err == nil {
					p.themeFiles[f] = themeFile{fi.ModTime(), newest}
				}
			}
		}
	}
	p.followTheme()
}

// setTheme writes a theme into a settings file, leaving the rest as it is.
func setTheme(path, theme string) bool {
	data, err := os.ReadFile(path)
	if err != nil || !gjson.ValidBytes(data) {
		return false
	}
	out, err := sjson.SetBytes(data, "theme", theme)
	if err != nil {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	tmp := path + ".argus-tmp"
	if os.WriteFile(tmp, out, fi.Mode().Perm()) != nil {
		return false
	}
	return os.Rename(tmp, path) == nil
}

// themeIsLight tells a light theme: a built-in one ("light", "light-ansi"…)
// or a custom one ("custom:<slug>") whose base is light.
func (p *Panel) themeIsLight(theme string) bool {
	if theme == argusTheme {
		return p.argusMode() == install.ThemeLight
	}
	if slug, ok := strings.CutPrefix(theme, "custom:"); ok {
		for _, f := range p.claudeSettings() {
			data, err := os.ReadFile(filepath.Join(filepath.Dir(f), "themes", slug+".json"))
			if err != nil {
				continue
			}
			return strings.HasPrefix(gjson.GetBytes(data, "base").String(), "light")
		}
		return false
	}
	return strings.HasPrefix(theme, "light")
}

// argusMode is the Argus theme's set chosen with T.
func (p *Panel) argusMode() string {
	for _, m := range install.ThemeModes {
		if m == p.prefs.ThemeMode {
			return m
		}
	}
	return install.ThemeDark
}

// themeMode is the set the app look draws with: the Argus theme's own set,
// or light or dark after another Claude theme. The classic look is dark.
func (p *Panel) themeMode() string {
	switch {
	case !p.appLook():
		return install.ThemeDark
	case p.claudeTheme == argusTheme:
		return p.argusMode()
	case p.themeIsLight(p.claudeTheme):
		return install.ThemeLight
	}
	return install.ThemeDark
}

// followTheme applies the Claude theme to the app look and to the editor.
func (p *Panel) followTheme() {
	mode := p.themeMode()
	p.setFreshTheme(mode)
	if mode == paletteMode {
		return
	}
	setPalette(mode)
	p.repaint()
}

// repaint redraws everything after the palette changed.
func (p *Panel) repaint() {
	p.prevLines = nil
	p.tabsText = ""
	p.titles = map[string]string{}
	if p.appLook() {
		p.applyLook()
	}
}

// setFreshTheme picks Argus's Fresh theme for the look (only when the
// editor uses one of Argus's themes, or none). Open editors take it when
// they start again.
func (p *Panel) setFreshTheme(mode string) {
	want := "argus-" + mode
	path := filepath.Join(p.d.Paths.Home, ".config", "fresh", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
		data = []byte("{}")
	}
	cur := gjson.GetBytes(data, "theme").String()
	if cur == want || (cur != "" && !strings.HasPrefix(cur, "argus")) {
		return
	}
	var v map[string]any
	if json.Unmarshal(data, &v) != nil {
		return
	}
	out, err := sjson.SetBytes(data, "theme", want)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, out, 0o644)
}

// argusTheme is Argus's theme as Claude names it in the settings.
const argusTheme = "custom:" + install.ClaudeThemeSlug

// writeArgusTheme makes the Argus theme file of every Claude account dark or
// light. Claude reloads a theme file when it changes: every open session on
// the Argus theme follows at once.
func (p *Panel) writeArgusTheme(mode string) {
	body := []byte(install.ClaudeTheme(mode))
	for _, pr := range p.d.Config.Profiles {
		if pr.Label() != history.Claude {
			continue
		}
		f := filepath.Join(pr.ClaudeConfigDir(p.d.Paths.Home), "themes", install.ClaudeThemeSlug+".json")
		if cur, err := os.ReadFile(f); err == nil && string(cur) == string(body) {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		_ = os.WriteFile(f, body, 0o644)
	}
}

// setArgusTheme puts the Argus theme on a set (picked in the T window), in
// Argus and in every Claude session using it, with no restart.
func (p *Panel) setArgusTheme(mode string) {
	p.prefs.ThemeMode = mode
	if mode == install.ThemeDark {
		p.prefs.ThemeMode = ""
	}
	_ = p.prefs.Save()
	p.writeArgusTheme(mode)
	// The accounts on another theme move to the Argus one (a session started
	// before that takes it when it restarts).
	for _, f := range p.claudeSettings() {
		if p.themeFiles[f].theme != argusTheme && setTheme(f, argusTheme) {
			if fi, err := os.Stat(f); err == nil {
				p.themeFiles[f] = themeFile{fi.ModTime(), argusTheme}
			}
		}
	}
	p.claudeTheme = argusTheme
	p.followTheme()
	p.flash(i18n.T("msg.theme." + mode))
}
