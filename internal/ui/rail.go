package ui

import (
	"time"

	"github.com/Marcone-Santos1/argus/internal/status"
)

// ---------- the list folded into a rail (key b) ----------
//
// The list shrinks to a few columns with one mark per session (state and
// agent), giving its room to the code and the chat. Keys and clicks work as
// in the full list; b opens it again.

const railW = 6

func (p *Panel) toggleRail() {
	p.rail = !p.rail
	p.applyWidth()
	p.prevLines = nil
}

func (p *Panel) viewRail() []string {
	w := p.w
	now := time.Now()
	body := []string{line(w, "", sp(" ≡", cQuiet)), ""}
	targets := []int{-1, -1}
	focus := 0
	for idx, it := range p.items {
		if it.agent < 0 {
			continue
		}
		a := p.agents[it.agent]
		sel := idx == p.sel
		bg, bar := "", sp(" ", "")
		if sel {
			bg, bar = aCard, sp("▌", aBlue)
			focus = len(body)
		}
		if a.Status == status.Waiting && a.Asleep.IsZero() {
			bg = aNeed
		}
		body = append(body, line(w, bg, bar, p.appDot(a, now), sp(" ", ""), p.agentGlyph(a.Profile)))
		targets = append(targets, idx)
	}
	return p.layoutClick(body, targets, []string{line(w, "", sp(" b", cFaint))}, focus)
}
