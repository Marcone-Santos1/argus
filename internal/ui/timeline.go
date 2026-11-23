package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

// ---------- today's timeline (key g) ----------

// stClosed marks, in the timeline, a session that was closed.
const stClosed = "closed"

// stateEvent is one state change, as saved in the day's timeline file.
type stateEvent struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"st"`
	At    int64  `json:"t"` // unix seconds
}

func (p *Panel) timelineFile(day time.Time) string {
	return filepath.Join(p.d.Paths.State, "_timeline", day.Format("2006-01-02")+".jsonl")
}

// logState appends a state change to the day's file (one small write, only
// when a session changes state).
func (p *Panel) logState(id, name, st string, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	path := p.timelineFile(at)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	data, _ := json.Marshal(stateEvent{ID: id, Name: name, State: st, At: at.Unix()})
	_, _ = f.Write(append(data, '\n'))
}

func readEvents(path string) []stateEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []stateEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e stateEvent
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.ID != "" {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// span of one state in the timeline.
type stateSpan struct {
	state    string
	from, to time.Time
}

// tlRow is one session in the timeline.
type tlRow struct {
	id, name string
	spans    []stateSpan
	waited   time.Duration
}

// timelineRows turns the day's events into one row per session that worked,
// each state lasting until the next change (or now, while it is open).
func timelineRows(events []stateEvent, now time.Time, alive map[string]bool) []tlRow {
	byID := map[string]*tlRow{}
	var order []string
	last := map[string]stateEvent{}
	closeSpan := func(id string, to time.Time) {
		prev, ok := last[id]
		if !ok || prev.State == stClosed {
			return
		}
		from := time.Unix(prev.At, 0)
		if to.After(from) {
			r := byID[id]
			r.spans = append(r.spans, stateSpan{prev.State, from, to})
			if prev.State == status.Waiting {
				r.waited += to.Sub(from)
			}
		}
	}
	for _, e := range events {
		if _, ok := byID[e.ID]; !ok {
			byID[e.ID] = &tlRow{id: e.ID}
			order = append(order, e.ID)
		}
		closeSpan(e.ID, time.Unix(e.At, 0))
		if e.Name != "" {
			byID[e.ID].name = e.Name
		}
		last[e.ID] = e
	}
	var out []tlRow
	for _, id := range order {
		end := now
		if !alive[id] {
			end = time.Unix(last[id].At, 0) // closed without a record: stop at its last change
		}
		closeSpan(id, end)
		r := byID[id]
		worked := false
		for _, s := range r.spans {
			if s.state == status.Working {
				worked = true
			}
		}
		if worked {
			out = append(out, *r)
		}
	}
	return out
}

// stateAt is the state that covers most of [from, to) in a row ("" = none).
func (r tlRow) stateAt(from, to time.Time) string {
	best, bestD := "", time.Duration(0)
	for _, s := range r.spans {
		a, b := s.from, s.to
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		if d := b.Sub(a); d > bestD {
			best, bestD = s.state, d
		}
	}
	return best
}

func (p *Panel) openTimeline() {
	p.mode = modeTimeline
	p.setZoom(true)
}

func (p *Panel) viewTimeline() []string {
	w, h := p.w, p.h
	now := time.Now()
	alive := map[string]bool{}
	names := map[string]string{}
	for _, a := range p.agents {
		alive[a.ID] = true
		names[a.ID] = p.sessionText(a)
	}
	rows := timelineRows(readEvents(p.timelineFile(now)), now, alive)
	body := []string{line(w, "", sp(" "+i18n.T("tl.title"), "\x1b[1m")), ""}
	foot := []string{line(w, "", sp(strings.Repeat("─", w), cFaint)), line(w, "", sp(" "+i18n.T("help.close.any"), cFaint))}
	if len(rows) == 0 {
		body = append(body, line(w, "", sp(" "+i18n.T("tl.empty"), cQuiet)))
		return layout(body, foot, h, 0)
	}
	start := now
	for _, r := range rows {
		if len(r.spans) > 0 && r.spans[0].from.Before(start) {
			start = r.spans[0].from
		}
	}
	start = start.Truncate(time.Hour)
	if now.Sub(start) < time.Hour {
		start = now.Add(-time.Hour).Truncate(time.Hour)
	}
	nameW := min(18, max(10, w/6))
	cols := max(w-nameW-3, 10)
	slot := now.Sub(start) / time.Duration(cols)

	// Hour labels over the bars.
	axis := []rune(strings.Repeat(" ", cols))
	for t := start; !t.After(now); t = t.Add(time.Hour) {
		c := int(t.Sub(start) / slot)
		label := []rune(fmt.Sprintf("%dh", t.Hour()))
		if c+len(label) <= cols {
			copy(axis[c:], label)
		}
	}
	body = append(body, line(w, "", sp(" "+strings.Repeat(" ", nameW)+" ", ""), sp(string(axis), cFaint)))

	var waited time.Duration
	var most tlRow
	for _, r := range rows {
		name := names[r.id]
		if name == "" {
			name = r.name
		}
		spans := []span{sp(" "+fit(name, nameW)+" ", "")}
		for c := 0; c < cols; c++ {
			from := start.Add(time.Duration(c) * slot)
			switch r.stateAt(from, from.Add(slot)) {
			case status.Working:
				spans = append(spans, sp("█", cWorking))
			case status.Waiting:
				spans = append(spans, sp("▌", cWaiting))
			case status.Done:
				spans = append(spans, sp("─", cDone))
			case status.Idle:
				spans = append(spans, sp("·", cFaint))
			default:
				spans = append(spans, sp(" ", ""))
			}
		}
		body = append(body, line(w, "", spans...))
		waited += r.waited
		if r.waited > most.waited {
			most = r
			if n := names[r.id]; n != "" {
				most.name = n
			}
		}
	}
	body = append(body, "",
		line(w, "", sp(" █ ", cWorking), sp(i18n.T("st.working"), cQuiet), sp("   ▌ ", cWaiting), sp(i18n.T("tl.waited"), cQuiet),
			sp("   ─ ", cDone), sp(i18n.T("st.done"), cQuiet), sp("   · ", cFaint), sp(i18n.T("st.idle"), cQuiet)))
	if waited >= time.Minute {
		sum := []span{sp(" "+i18n.T("tl.waitedToday")+" ", cQuiet), sp(durText(waited), cWaiting)}
		if most.waited >= time.Minute {
			sum = append(sum, sp(" · "+i18n.T("tl.most", most.name, durText(most.waited)), cFaint))
		}
		body = append(body, line(w, "", sum...))
	}
	return layout(body, foot, h, 0)
}

// durText is "38 min" or "2h05".
func durText(d time.Duration) string {
	m := int(d.Minutes())
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}
