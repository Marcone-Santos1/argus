package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/gitinfo"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/usage"
)

// gitState is a folder's git info; ok is false outside a repository.
type gitState struct {
	gitinfo.Info
	ok bool
}

// ---------- git and last reply (read when a turn ends) ----------

// refreshExtras reads, off the main loop, the session folder's git state and
// Claude's last reply. Called when a turn ends and when a session is first seen.
func (p *Panel) refreshExtras(a agent) {
	projects := p.d.Paths.ClaudeProjects()
	claude := !p.isOpencode(a)
	ch := p.asyncC
	go func() {
		g, ok := gitinfo.Read(a.Dir)
		reply := ""
		if claude && a.Session != "" {
			reply = history.LastReply(projects, a.Session)
		}
		ch <- func() {
			p.git[a.Dir] = gitState{g, ok}
			if reply != "" {
				p.reply[a.ID] = reply
			}
		}
	}()
}

// gitText is the short git state for a session's second line: "⎇ main ✎3 ↑1".
func (p *Panel) gitText(dir string) string {
	g, ok := p.git[dir]
	if !ok || !g.ok {
		return ""
	}
	s := "⎇ " + g.Branch
	if g.Changed > 0 {
		s += fmt.Sprintf(" ✎%d", g.Changed)
	}
	if g.Ahead > 0 {
		s += fmt.Sprintf(" ↑%d", g.Ahead)
	}
	return s
}

// ---------- account usage (from the status line) ----------

func (p *Panel) readUsage(path string) {
	id := filepath.Base(path)
	if in, ok := usage.Read(path); ok {
		p.usage[id] = in
		for i := range p.agents {
			if p.agents[i].ID == id {
				p.syncTitle(i)
			}
		}
	}
}

func (p *Panel) loadUsage() {
	entries, _ := os.ReadDir(p.d.Paths.Usage())
	for _, e := range entries {
		p.readUsage(filepath.Join(p.d.Paths.Usage(), e.Name()))
	}
}

// profileUsage is the newest report of any session of the profile.
func (p *Panel) profileUsage(profile string) (usage.Info, bool) {
	var best usage.Info
	found := false
	for _, a := range p.agents {
		in, ok := p.usage[a.ID]
		if ok && a.Profile == profile && in.FiveHour != nil && (!found || in.At.After(best.At)) {
			best, found = in, true
		}
	}
	return best, found
}

func pctColor(pct float64) string {
	switch {
	case pct >= 80:
		return cEnded
	case pct >= 50:
		return cWorking
	}
	return ""
}

// usageGroups are the footer's account parts, one list per account: "pes 5h
// ▰▱▱▱▱ 21% 2h48" and "7d ▰▱▱▱▱ 8% 4d6h", with the time left until each
// window resets.
func (p *Panel) usageGroups(now time.Time) [][][]span {
	var out [][][]span
	for _, pr := range p.d.Config.Profiles {
		in, ok := p.profileUsage(pr.Name)
		if !ok {
			continue
		}
		var gs [][]span
		add := func(label string, l *usage.Limit) {
			if l == nil {
				return
			}
			pct, left := l.Percent, ""
			if l.Expired(now) {
				pct = 0
			} else if !l.Resets.IsZero() {
				left = fmt.Sprintf(" %-5s", untilText(l.Resets.Sub(now)))
			}
			color := pctColor(pct)
			barColor := color
			if barColor == "" {
				barColor = cDone
			}
			on, off := cells(pct, 5)
			gs = append(gs, []span{sp(label+" ", cFaint), sp(on, barColor), sp(off, cFaint),
				sp(fmt.Sprintf(" %2.0f%%", pct), color), sp(left, cQuiet)})
		}
		add("5h", in.FiveHour)
		add("7d", in.SevenDay)
		if len(gs) > 0 {
			gs[0] = append([]span{sp(pr.Tag()+" ", p.accountColor(pr.Name))}, gs[0]...)
			out = append(out, gs)
		}
	}
	return out
}

// untilText is a short countdown: "35m", "2h48", "4d6h".
func untilText(d time.Duration) string {
	m := int(d.Minutes())
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", max(m, 0))
	case m < 24*60:
		return fmt.Sprintf("%dh%02d", m/60, m%60)
	}
	return fmt.Sprintf("%dd%dh", m/(24*60), m%(24*60)/60)
}

// ---------- templates (key t) ----------

func (p *Panel) openTemplates() {
	p.tplFrom = ""
	if a := p.current(); a != nil {
		p.tplFrom = a.ID
	}
	p.tplSel = 0
	p.mode = modeTemplates
}

// tplCount is the number of rows: the templates and, when a session is
// selected, "save it as a template".
func (p *Panel) tplCount() int {
	n := len(p.d.Config.Templates)
	if p.tplFrom != "" {
		n++
	}
	return n
}

func (p *Panel) keyTemplates(k string) {
	switch k {
	case "esc", "ctrl-C", "q":
		p.closePicker()
	case "up", "k":
		if p.tplSel > 0 {
			p.tplSel--
		}
	case "down", "j":
		if p.tplSel < p.tplCount()-1 {
			p.tplSel++
		}
	case "enter":
		p.pickTemplate(p.tplSel)
	case "x":
		p.deleteTemplate(p.tplSel)
	default:
		if len(k) == 1 && k >= "1" && k <= "9" {
			if i := int(k[0] - '1'); i < len(p.d.Config.Templates) {
				p.pickTemplate(i)
			}
		}
	}
}

func (p *Panel) pickTemplate(i int) {
	ts := p.d.Config.Templates
	if i == len(ts) && p.tplFrom != "" {
		p.saveTemplate()
		return
	}
	if i < 0 || i >= len(ts) {
		return
	}
	t := ts[i]
	prof, ok := p.profileOf(t.Profile)
	if !ok {
		prof, ok = p.profileOf(p.d.Config.DefaultProfile(t.Dir, p.prefs))
	}
	if fi, err := os.Stat(t.Dir); err != nil || !fi.IsDir() || !ok {
		p.flash(i18n.T("msg.tplBroken", t.Name))
		return
	}
	p.closePicker()
	p.firstPrompt = t.Prompt
	p.worktreeOf = ""
	p.startAgent(t.Dir, prof, nil)
}

// saveTemplate turns the selected session (folder and profile) into a
// template, asking for its name and first message.
func (p *Panel) saveTemplate() {
	var a *agent
	for i := range p.agents {
		if p.agents[i].ID == p.tplFrom {
			a = &p.agents[i]
		}
	}
	if a == nil {
		return
	}
	dir, prof := a.Dir, a.Profile
	p.closePicker()
	p.ask(i18n.T("tpl.askName"), filepath.Base(dir), func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		p.ask(i18n.T("tpl.askPrompt"), "", func(prompt string) {
			p.d.Config.Templates = append(p.d.Config.Templates,
				config.Template{Name: name, Dir: dir, Profile: prof, Prompt: strings.TrimSpace(prompt), Saved: true})
			if err := config.SaveTemplates(p.d.Paths.Templates(), p.d.Config.Templates); err != nil {
				p.flash(err.Error())
				return
			}
			p.flash(i18n.T("msg.tplSaved", name))
		})
	})
}

func (p *Panel) deleteTemplate(i int) {
	ts := p.d.Config.Templates
	if i < 0 || i >= len(ts) {
		return
	}
	if !ts[i].Saved {
		p.flash(i18n.T("msg.tplInConfig"))
		return
	}
	name := ts[i].Name
	p.confirm(i18n.T("msg.tplDelete", name), func() {
		p.d.Config.Templates = append(ts[:i:i], ts[i+1:]...)
		if err := config.SaveTemplates(p.d.Paths.Templates(), p.d.Config.Templates); err != nil {
			p.flash(err.Error())
		}
		p.mode = modeTemplates
		p.tplSel = clamp(p.tplSel, p.tplCount())
	})
}

func (p *Panel) viewTemplates() []string {
	w := p.w
	body := []string{line(w, "", sp(" "+i18n.T("tpl.title"), "\x1b[1m")), ""}
	ts := p.d.Config.Templates
	if len(ts) == 0 {
		body = append(body, line(w, "", sp(i18n.T("tpl.empty1"), cQuiet)), line(w, "", sp(i18n.T("tpl.empty2"), cQuiet)), "")
	}
	first := len(body)
	for i, t := range ts {
		bg, bar := rowBar(i == p.tplSel, false)
		n := " "
		if i < 9 {
			n = fmt.Sprint(i + 1)
		}
		where := p.d.Paths.Pretty(t.Dir)
		body = append(body, line(w, bg, bar, sp(" "+n+"  ", cQuiet), sp(t.Name, "\x1b[1m"), sp("  ", ""),
			sp(p.tag(t.Profile), p.accountColor(t.Profile)), sp(" · "+where, cFaint)))
	}
	if p.tplFrom != "" {
		bg, bar := rowBar(p.tplSel == len(ts), false)
		body = append(body, line(w, bg, bar, sp(" +  ", cAccent), sp(i18n.T("tpl.saveCurrent"), cAccent)))
	}
	foot := []string{line(w, "", sp(strings.Repeat("─", w), cFaint))}
	if p.tplSel < len(ts) && ts[p.tplSel].Prompt != "" {
		foot = append(foot, line(w, "", sp(" ❝ "+ts[p.tplSel].Prompt, cQuiet)))
	}
	for _, h := range wrapHints(tr("hint.pick", "hint.open", "hint.tplDelete", "hint.back"), w) {
		foot = append(foot, line(w, "", sp(stripANSI(h), cFaint)))
	}
	return p.layoutClick(body, listTargets(first, p.tplCount()), foot, p.tplSel+first)
}

// ---------- search inside conversations (history, h) ----------

// scheduleSearch starts a content search shortly after the typing stops; a
// newer search cancels the older one.
func (p *Panel) scheduleSearch() {
	if p.searchCancel != nil {
		p.searchCancel()
		p.searchCancel = nil
	}
	q := strings.TrimSpace(p.filter)
	p.contentHits = nil
	p.searching = len(q) >= 3
	if !p.searching {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.searchCancel = cancel
	h, ch := p.hist, p.asyncC
	go func() {
		select {
		case <-time.After(350 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		hits := h.Search(ctx, q)
		if ctx.Err() != nil {
			return
		}
		ch <- func() {
			if strings.TrimSpace(p.filter) != q || p.mode != modeHistory {
				return
			}
			p.searching = false
			p.contentHits = map[string]string{}
			for _, x := range hits {
				p.contentHits[x.Key] = x.Snippet
			}
			p.applyHistFilter()
		}
	}()
}

// stopSearch cancels a running search (leaving the history).
func (p *Panel) stopSearch() {
	if p.searchCancel != nil {
		p.searchCancel()
		p.searchCancel = nil
	}
	p.searching = false
	p.contentHits = nil
}

// isContentHit reports a conversation found by its text, not its title.
func (p *Panel) isContentHit(e history.Entry) bool {
	_, ok := p.contentHits[e.Key()]
	return ok && !matchTerms(e.Dir+" "+e.Title+" "+e.Profile+" "+e.Tool, p.filter)
}

// replyShown reports whether the session's last reply goes under it.
func replyShown(a agent) bool {
	return a.Status == status.Done || a.Status == status.Idle
}
