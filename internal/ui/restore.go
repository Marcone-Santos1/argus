package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// snapEntry is one open session, as saved for a restore after the tmux server
// goes away (a reboot, a crash, a kill-server).
type snapEntry struct {
	Name     string `json:"name"`
	Profile  string `json:"profile"`
	Dir      string `json:"dir"`
	Tool     string `json:"tool"`
	Session  string `json:"session,omitempty"` // conversation id, to resume it
	Worktree string `json:"worktree,omitempty"`
	Code     bool   `json:"code,omitempty"`   // its code column was open
	Asleep   int64  `json:"asleep,omitempty"` // hibernated since (unix): it comes back hibernated
}

func (p *Panel) snapshot() []snapEntry {
	out := make([]snapEntry, 0, len(p.agents))
	for _, a := range p.agents {
		tool := ""
		if pr, ok := p.profileOf(a.Profile); ok {
			tool = pr.Label()
		}
		e := snapEntry{Name: a.Name, Profile: a.Profile, Dir: a.Dir, Tool: tool, Session: a.Session, Worktree: a.Worktree, Code: p.codeOpen[a.ID]}
		if !a.Asleep.IsZero() {
			e.Asleep = a.Asleep.Unix()
		}
		out = append(out, e)
	}
	return out
}

// saveSnapshot writes the open sessions, only when they changed. It is
// skipped while a restore is being offered, so the offer is not erased.
func (p *Panel) saveSnapshot() {
	// Before the panel decides whether to offer a restore, an empty session
	// list means "not started yet", not "everything was closed".
	if !p.snapReady || p.restoreOffer != nil {
		return
	}
	data, err := json.Marshal(p.snapshot())
	if err != nil || bytes.Equal(data, p.lastSnap) {
		return
	}
	path := p.d.Paths.Snapshot()
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil && os.Rename(tmp, path) == nil {
		p.lastSnap = data
	}
}

// loadSnapshot reads the saved sessions that can still be reopened: the
// profile still exists and the folder is still there.
func loadSnapshot(path string, cfg *config.Config) []snapEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var all []snapEntry
	if json.Unmarshal(data, &all) != nil {
		return nil
	}
	var ok []snapEntry
	for _, e := range all {
		if _, found := cfg.Profile(e.Profile); !found {
			continue
		}
		if fi, err := os.Stat(e.Dir); err != nil || !fi.IsDir() {
			continue
		}
		ok = append(ok, e)
	}
	return ok
}

// offerRestore asks, on a fresh server with no sessions, whether to reopen
// the sessions that were open before.
func (p *Panel) offerRestore() {
	p.snapReady = true
	if len(p.agents) > 0 {
		p.saveSnapshot()
		return
	}
	snap := loadSnapshot(p.d.Paths.Snapshot(), p.d.Config)
	if len(snap) == 0 {
		return
	}
	p.restoreOffer = snap
	p.confirmNo = func() {
		p.restoreOffer = nil
		p.saveSnapshot()
	}
	p.confirm(i18n.T("msg.restoreAsk", len(snap)), func() {
		offer := p.restoreOffer
		p.restoreOffer = nil
		p.restore(offer)
	})
}

// restore reopens the sessions, resuming each conversation in its account.
// dedupe keeps one entry per conversation, the awake one when there is one:
// a list saved with the same conversation twice would start it twice.
func dedupe(snap []snapEntry) []snapEntry {
	awake := map[string]bool{}
	for _, e := range snap {
		if e.Session != "" && e.Asleep == 0 {
			awake[e.Tool+":"+e.Session] = true
		}
	}
	seen := map[string]bool{}
	out := snap[:0:0]
	for _, e := range snap {
		k := e.Tool + ":" + e.Session
		if e.Session != "" {
			if seen[k] || (e.Asleep > 0 && awake[k]) {
				continue
			}
			seen[k] = true
		}
		out = append(out, e)
	}
	return out
}

func (p *Panel) restore(snap []snapEntry) {
	first, n := "", 0
	for _, e := range dedupe(snap) {
		prof, ok := p.profileOf(e.Profile)
		if !ok {
			continue
		}
		var resume *history.Entry
		if e.Session != "" && history.SafeID(e.Session) {
			tool := e.Tool
			if tool == "" {
				tool = prof.Label()
			}
			resume = &history.Entry{Tool: tool, ID: e.Session}
		}
		// A hibernated session comes back hibernated: its pane holds the
		// note, and Enter resumes the conversation as before the restart.
		var asleep time.Time
		if e.Asleep > 0 && resume != nil && prof.Label() == history.Claude {
			asleep = time.Unix(e.Asleep, 0)
		}
		id, err := p.spawn(e.Dir, prof, resume, e.Worktree, e.Name, asleep)
		if err != nil {
			p.flash(err.Error())
			continue
		}
		if e.Code {
			p.codeOpen[id] = true // the column comes back with the session
		}
		if first == "" && asleep.IsZero() { // focusing a hibernated one would wake it
			first = id
		}
		n++
	}
	p.reconcile()
	if first != "" {
		p.selectKey("a:" + first)
		p.focusSession()
	}
	p.flash(i18n.T("msg.restored", n))
}

// spawn starts a session in a hidden tmux window and returns its id. It does
// not touch the selection or the right side.
//
// With asleep set, the session starts hibernated: no process, only the note.
func (p *Panel) spawn(dir string, prof config.Profile, resume *history.Entry, wtRepo, name string, asleep time.Time) (string, error) {
	session := ""
	if resume != nil {
		if !history.SafeID(resume.ID) {
			return "", fmt.Errorf("%s", i18n.T("msg.badID"))
		}
		session = resume.ID
	}
	id := newID()
	if name == "" {
		name = filepath.Base(dir)
		taken := map[string]bool{}
		for _, a := range p.agents {
			taken[a.Name] = true
		}
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s %d", filepath.Base(dir), n)
		}
	}
	_ = status.Write(p.d.Paths.Status(id), status.Idle)

	env, cmd := p.launch(id, prof, resume)
	opts := []string{"remain-on-exit", "off"}
	if !asleep.IsZero() {
		// The note stays a second so the options below land before it exits.
		cmd = []string{"/bin/sh", "-c", sleepNote(asleep) + "; sleep 1"}
		opts = []string{"remain-on-exit", "on", "remain-on-exit-format", " ", "@argus_sleep", fmt.Sprint(asleep.Unix())}
	}
	args := []string{"new-window", "-d", "-t", tmux.Session + ":", "-n", id, "-c", dir}
	args = append(args, env...)
	args = append(args, "-P", "-F", "#{pane_id}")
	paneID, err := p.d.Tmux.Run(append(args, cmd...)...)
	if err != nil {
		return "", err
	}
	opts = append([]string{"@argus_role", tmux.RoleAgent, "@argus_id", id, "@argus_profile", prof.Name,
		"@argus_dir", dir, "@argus_name", name, "@argus_created", fmt.Sprint(time.Now().UnixNano()),
		"@argus_session", session, "@argus_worktree", wtRepo}, opts...)
	if err := p.d.Tmux.SetPaneOptions(paneID, opts...); err != nil {
		return id, err
	}
	return id, nil
}
