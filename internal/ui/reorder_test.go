package ui

import (
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/history"
)

func TestMoveSession(t *testing.T) {
	p := testPanel(t)
	p.agents = []agent{
		{ID: "a1", Dir: "/a", Created: 10},
		{ID: "b1", Dir: "/b", Created: 20},
		{ID: "a2", Dir: "/a", Created: 30},
		{ID: "c1", Dir: "/c", Created: 40},
	}
	p.rebuildItems("")
	order := func() string {
		var ids []string
		for _, it := range p.items {
			if it.agent >= 0 {
				ids = append(ids, p.agents[it.agent].ID)
			}
		}
		return strings.Join(ids, " ")
	}
	if got := order(); got != "a1 a2 b1 c1" {
		t.Fatalf("start: %s", got)
	}
	p.selectKey("a:a2")
	p.moveSession(-1) // inside its folder
	if got := order(); got != "a2 a1 b1 c1" {
		t.Fatalf("up inside folder: %s", got)
	}
	p.moveSession(-1) // already first of all: nothing
	if got := order(); got != "a2 a1 b1 c1" {
		t.Fatalf("top: %s", got)
	}
	p.selectKey("a:b1")
	p.moveSession(-1) // folder's edge: /b goes above /a
	if got := order(); got != "b1 a2 a1 c1" {
		t.Fatalf("folder up: %s", got)
	}
	if p.selKey() != "a:b1" {
		t.Fatalf("selection lost: %s", p.selKey())
	}
	p.moveSession(1) // and back down
	p.selectKey("a:c1")
	p.moveSession(-1)
	if got := order(); got != "a2 a1 c1 b1" {
		t.Fatalf("c up: %s", got)
	}
	// A new session in a known folder joins it; in a new folder, goes last.
	p.agents = append(p.agents, agent{ID: "a3", Dir: "/a", Created: 50}, agent{ID: "d1", Dir: "/d", Created: 60})
	sortAgents(p.agents)
	p.rebuildItems("")
	if got := order(); got != "a2 a1 a3 c1 b1 d1" {
		t.Fatalf("new sessions: %s", got)
	}
}

func TestRememberedName(t *testing.T) {
	p := testPanel(t)
	p.prefs.Names["conv-1"] = "ficar rico"
	es := p.withRemembered([]history.Entry{{ID: "conv-1", Title: "Simulação"}, {ID: "conv-2", Title: "Outra"}})
	if es[0].Title != "ficar rico" || es[1].Title != "Outra" {
		t.Fatalf("titles: %q %q", es[0].Title, es[1].Title)
	}
}
