package ui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

// ---------- hibernation: a finished session left alone gives its memory back ----------

// stAsleep is the list state of a hibernated session: its process is closed,
// its pane stays (dead, kept by remain-on-exit) with the conversation id.
const stAsleep = "asleep"

// canHibernate: a finished Claude session not in sight, untouched for the
// configured time, with nothing running behind it and a conversation to
// resume.
func (p *Panel) canHibernate(a agent, now time.Time, after time.Duration) bool {
	if after <= 0 || !a.Asleep.IsZero() || a.Displayed || a.Since.IsZero() || now.Sub(a.Since) < after {
		return false
	}
	if now.Sub(p.seenAt[a.ID]) < after { // looked at recently counts as use
		return false
	}
	if a.Status != status.Done && a.Status != status.Idle {
		return false
	}
	if p.shells[a.ID] > 0 || a.Subagents > 0 || !history.SafeID(a.Session) {
		return false
	}
	prof, ok := p.profileOf(a.Profile)
	return ok && prof.Label() == history.Claude
}

// hibernateIdle runs on the tick: it only compares times, and calls tmux
// only for a session that is due.
func (p *Panel) hibernateIdle(now time.Time) {
	after := p.d.Config.Hibernate()
	changed := false
	for _, a := range p.agents {
		if a.Displayed {
			p.seenAt[a.ID] = now
		}
	}
	for i := range p.agents {
		if p.canHibernate(p.agents[i], now, after) {
			if p.hibernate(&p.agents[i], now) {
				changed = true
			}
		}
	}
	if changed {
		p.reconcile()
	}
}

// hibernate closes the session's process; the pane stays, dead, with a note.
func (p *Panel) hibernate(a *agent, now time.Time) bool {
	if err := p.d.Tmux.SetPaneOptions(a.Pane,
		"remain-on-exit", "on", "remain-on-exit-format", " ",
		"@argus_sleep", fmt.Sprint(now.Unix())); err != nil {
		return false
	}
	note := fmt.Sprintf(`printf '\033[2J\033[H\n  %%s\n  %%s\n' %s %s`,
		shellQuote(i18n.T("sleep.title", status.Short(now.Sub(a.Since)))), shellQuote(i18n.T("sleep.how")))
	if _, err := p.d.Tmux.Run("respawn-pane", "-k", "-t", a.Pane, note); err != nil {
		_, _ = p.d.Tmux.Run("set", "-pu", "-t", a.Pane, "@argus_sleep")
		return false
	}
	a.Asleep = now
	return true
}

// wake resumes a hibernated session's conversation in its own pane.
func (p *Panel) wake(a *agent) bool {
	prof, ok := p.profileOf(a.Profile)
	if !ok || !history.SafeID(a.Session) {
		p.flash(i18n.T("msg.noProfile", a.Profile))
		return false
	}
	_ = status.Write(p.d.Paths.Status(a.ID), status.Idle)
	env, cmd := p.launch(a.ID, prof, &history.Entry{Tool: history.Claude, ID: a.Session})
	args := append([]string{"respawn-pane", "-k", "-t", a.Pane, "-c", a.Dir}, env...)
	if _, err := p.d.Tmux.Run(append(args, cmd...)...); err != nil {
		p.flash(err.Error())
		return false
	}
	_, _ = p.d.Tmux.Run("set", "-p", "-t", a.Pane, "remain-on-exit", "off", ";", "set", "-pu", "-t", a.Pane, "@argus_sleep")
	a.Asleep = time.Time{}
	return true
}

// awake counts the sessions with a running process.
func (p *Panel) awake() int {
	n := 0
	for _, a := range p.agents {
		if a.Asleep.IsZero() {
			n++
		}
	}
	return n
}

// ---------- background agents (Claude Code's subagents run in the background) ----------

// backgroundAgents counts a Claude session's background agents still running.
func (p *Panel) backgroundAgents(a agent) int {
	prof, ok := p.profileOf(a.Profile)
	if !ok || prof.Label() != history.Claude || a.Session == "" {
		return 0
	}
	path, ok := p.transcripts[a.Session]
	if !ok { // found once per conversation: the glob reads every project folder
		path = history.TranscriptPath(filepath.Join(prof.ClaudeConfigDir(p.d.Paths.Home), "projects"), a.Session)
		if path != "" {
			p.transcripts[a.Session] = path
		}
	}
	return history.BackgroundAgentsOf(path, time.Now())
}

// bgCheck re-reads, every 10 s, the sessions held working by background
// agents, so they turn done when the last one ends.
func (p *Panel) bgCheck() {
	if len(p.bgAgents) == 0 {
		return
	}
	if p.bgTick++; p.bgTick%5 != 0 {
		return
	}
	for i, a := range p.agents {
		if p.bgAgents[a.ID] > 0 {
			p.refreshStatus(i, true)
			p.syncTitle(i)
		}
	}
}

// bgSoon counts a session's background agents off the main loop (on start)
// and re-reads its state when some are running.
func (p *Panel) bgSoon(id string) {
	var a agent
	for _, x := range p.agents {
		if x.ID == id {
			a = x
		}
	}
	prof, ok := p.profileOf(a.Profile)
	if !ok || prof.Label() != history.Claude || a.Session == "" {
		return
	}
	projects := filepath.Join(prof.ClaudeConfigDir(p.d.Paths.Home), "projects")
	ch := p.asyncC
	go func() {
		path := history.TranscriptPath(projects, a.Session)
		n := history.BackgroundAgentsOf(path, time.Now())
		ch <- func() {
			if path != "" {
				p.transcripts[a.Session] = path
			}
			if n == 0 {
				return
			}
			p.bgAgents[id] = n
			for i := range p.agents {
				if p.agents[i].ID == id {
					p.refreshStatus(i, false)
					p.syncTitle(i)
				}
			}
		}
	}()
}
