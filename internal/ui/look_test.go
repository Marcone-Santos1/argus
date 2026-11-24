package ui

import (
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/status"
)

func TestAppLook(t *testing.T) {
	p := testPanel(t)
	p.w, p.h = 60, 40
	p.prefs.Look = lookApp
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Name: "Api work", Status: status.Done, Created: 1},
		{ID: "b1", Dir: "/p/site", Name: "Site fix", Status: status.Waiting, Created: 2},
		{ID: "c1", Dir: "/p/api", Name: "Old one", Status: status.Done, Created: 3},
	}
	p.agents[2].Asleep = p.agents[2].Asleep.Add(1) // hibernated
	p.rebuildItems("")
	var keys []string
	for i := range p.items {
		keys = append(keys, p.itemKey(i))
	}
	if got := strings.Join(keys, " "); got != "a:b1 a:a1 f:"+asleepKey {
		t.Fatalf("items: %q", got)
	}
	screen := stripANSI(strings.Join(p.frame(), "\n"))
	for _, want := range []string{"Site fix", "Api work", "Old one", "◌"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q in:\n%s", want, screen)
		}
	}
	// Opening the hibernated group lists its sessions.
	p.selectKey("f:" + asleepKey)
	p.handleKey("enter")
	if p.itemKey(len(p.items)-1) != "a:c1" {
		t.Fatalf("asleep not opened: %v", p.items)
	}
	// L goes back to the classic list, in creation order by folder.
	p.handleKey("L")
	if p.appLook() || p.itemKey(0) != "a:a1" {
		t.Fatalf("classic: look=%q first=%s", p.prefs.Look, p.itemKey(0))
	}
}
