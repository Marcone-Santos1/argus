package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

// ---------- keeping a long conversation small ----------
//
// Every turn sends the whole conversation again, so a session near its
// context limit costs far more per message than a fresh one, and Claude
// Code's own compact fires when the window is full: usually mid-task. Argus
// compacts between turns instead (compact_at), and C hands the work over to
// a new conversation in the same place.

// settle is how long after a turn ends Argus waits before typing into the
// session: the status line reports the context a moment after the Stop
// hook, and the prompt box needs to be back.
var settle = 1500 * time.Millisecond

// turnEnded runs when a session goes from working to done.
func (p *Panel) turnEnded(id string) {
	ch := p.asyncC
	go func() {
		time.Sleep(settle)
		ch <- func() {
			if _, ok := p.handoffs[id]; ok {
				p.finishHandoff(id)
				return
			}
			p.autoCompact(id)
		}
	}()
}

// agentByID is the session with this id (nil when it is gone).
func (p *Panel) agentByID(id string) *agent {
	for i := range p.agents {
		if p.agents[i].ID == id {
			return &p.agents[i]
		}
	}
	return nil
}

// isClaude: compact and handoff are Claude Code commands.
func (p *Panel) isClaude(a agent) (config.Profile, bool) {
	prof, ok := p.profileOf(a.Profile)
	return prof, ok && prof.Label() == history.Claude
}

// typeInto types a line into a session's prompt and submits it. Enter goes
// a moment later: sent with the text, the TUI takes it as part of a paste.
func (p *Panel) typeInto(pane, text string) error {
	if _, err := p.d.Tmux.Run("send-keys", "-t", pane, "-l", text); err != nil {
		return err
	}
	tc := p.d.Tmux
	go func() {
		time.Sleep(400 * time.Millisecond)
		_, _ = tc.Run("send-keys", "-t", pane, "Enter")
	}()
	return nil
}

// shouldCompact: a Claude session that finished a turn above the threshold,
// out of sight, with a context reading taken after the last compact.
func (p *Panel) shouldCompact(a agent) bool {
	at := p.d.Config.CompactAt
	if at <= 0 || a.Status != status.Done || !a.Asleep.IsZero() || p.bgAgents[a.ID] > 0 {
		return false
	}
	if _, ok := p.isClaude(a); !ok {
		return false
	}
	in, ok := p.usage[a.ID]
	if !ok || in.Context < float64(at) || !in.At.After(p.compacted[a.ID]) {
		return false
	}
	return !p.focusedOn(a)
}

func (p *Panel) autoCompact(id string) {
	a := p.agentByID(id)
	if a == nil || !p.shouldCompact(*a) {
		return
	}
	pct := p.usage[id].Context
	if p.compact(a) {
		p.flash(i18n.T("msg.compacting", a.Name, int(pct)))
	}
}

// compact sends /compact with the configured focus.
func (p *Panel) compact(a *agent) bool {
	if err := p.typeInto(a.Pane, "/compact "+p.d.Config.CompactFocus); err != nil {
		p.flash(err.Error())
		return false
	}
	p.compacted[a.ID] = time.Now()
	return true
}

// compactSelected is c: compact the selected session now.
func (p *Panel) compactSelected() {
	a := p.current()
	if a == nil {
		return
	}
	if _, ok := p.isClaude(*a); !ok {
		p.flash(i18n.T("msg.onlyClaude"))
		return
	}
	if !a.Asleep.IsZero() || a.Status == status.Working || a.Status == status.Waiting {
		p.flash(i18n.T("msg.notIdle", a.Name))
		return
	}
	if p.compact(a) {
		p.flash(i18n.T("msg.compacting", a.Name, int(p.usage[a.ID].Context)))
	}
}

// ---------- C: continue in a new conversation ----------

// handoffFile is where a session's handoff goes by default.
func (p *Panel) handoffFile(id string) string {
	return filepath.Join(p.d.Paths.State, "_handoff", id+".md")
}

func fillHandoff(tmpl, file, name string) string {
	return strings.NewReplacer("{file}", file, "{name}", name).Replace(tmpl)
}

// handoffSelected is C: the session writes its handoff; when that turn ends
// a new conversation starts in its pane (finishHandoff). C again cancels.
func (p *Panel) handoffSelected() {
	a := p.current()
	if a == nil {
		return
	}
	if _, ok := p.handoffs[a.ID]; ok {
		delete(p.handoffs, a.ID)
		p.flash(i18n.T("msg.handoffCancel", a.Name))
		return
	}
	if _, ok := p.isClaude(*a); !ok {
		p.flash(i18n.T("msg.onlyClaude"))
		return
	}
	if !a.Asleep.IsZero() || a.Status == status.Working || a.Status == status.Waiting {
		p.flash(i18n.T("msg.notIdle", a.Name))
		return
	}
	id, name := a.ID, a.Name
	p.confirm(i18n.T("msg.handoffAsk", name), func() {
		a := p.agentByID(id)
		if a == nil {
			return
		}
		file := p.handoffFile(id)
		_ = os.MkdirAll(filepath.Dir(file), 0o755)
		_ = os.Remove(file) // an old one must not pass for this one
		if err := p.typeInto(a.Pane, fillHandoff(p.d.Config.HandoffPrompt, file, name)); err != nil {
			p.flash(err.Error())
			return
		}
		p.handoffs[id] = file
		p.flash(i18n.T("msg.handoffWriting", name))
	})
}

// finishHandoff starts the new conversation in the session's own pane: same
// place in the list, name, folder and code column.
func (p *Panel) finishHandoff(id string) {
	file := p.handoffs[id]
	delete(p.handoffs, id)
	a := p.agentByID(id)
	if a == nil {
		return
	}
	prof, ok := p.isClaude(*a)
	if !ok {
		return
	}
	if strings.Contains(p.d.Config.HandoffPrompt, "{file}") {
		if fi, err := os.Stat(file); err != nil || fi.Size() == 0 {
			p.flash(i18n.T("msg.handoffMissing", a.Name))
			return
		}
	}
	p.firstPrompt = fillHandoff(p.d.Config.HandoffResume, file, a.Name)
	env, cmd := p.launch(id, prof, nil)
	p.firstPrompt = ""
	args := append([]string{"respawn-pane", "-k", "-t", a.Pane, "-c", a.Dir}, env...)
	if _, err := p.d.Tmux.Run(append(args, cmd...)...); err != nil {
		p.flash(err.Error())
		return
	}
	// The hook reports the new conversation's id on its first event.
	_ = p.d.Tmux.SetPaneOptions(a.Pane, "@argus_session", "")
	_ = os.WriteFile(p.d.Paths.Status(id), []byte(status.Working+"\n"), 0o644)
	delete(p.usage, id)
	_ = os.Remove(filepath.Join(p.d.Paths.Usage(), id))
	delete(p.compacted, id)
	p.flash(i18n.T("msg.handoffDone", a.Name))
	p.reconcile()
}
