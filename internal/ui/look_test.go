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
	if got := strings.Join(keys, " "); got != "a:b1 g:/p/api a:a1 f:"+asleepKey {
		t.Fatalf("items: %q", got)
	}
	screen := stripANSI(strings.Join(p.frame(), "\n"))
	for _, want := range []string{"Site fix", "Api work", "Old one", "◌"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q in:\n%s", want, screen)
		}
	}
	// Enter on a project's heading collapses it, and again opens it.
	p.selectKey("g:/p/api")
	p.handleKey("enter")
	shown := func(k string) bool {
		for i := range p.items {
			if p.itemKey(i) == k {
				return true
			}
		}
		return false
	}
	if shown("a:a1") || !p.prefs.Collapsed["/p/api"] {
		t.Fatalf("collapsed: %v", p.items)
	}
	p.selectKey("g:/p/api")
	p.handleKey("enter")
	if !shown("a:a1") {
		t.Fatalf("opened again: %v", p.items)
	}
	// Selecting a session of a collapsed project (a tab, the overview) opens it.
	p.selectKey("g:/p/api")
	p.handleKey("enter")
	if !p.selectKey("a:a1") || p.prefs.Collapsed["/p/api"] {
		t.Fatalf("select opens the project: %v", p.prefs.Collapsed)
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

func TestTmuxSpans(t *testing.T) {
	got := tmuxSpans([]span{sp("5h ", cFaint), sp("12% #1", "\x1b[1m"+aAmber)})
	want := "#[default,fg=#4a4f59]5h #[default,fg=#f5b14c,bold]12% ##1"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if s := stripTmux(got); s != "5h 12% #1" {
		t.Fatalf("strip: %q", s)
	}
}

func TestTabsLine(t *testing.T) {
	p := testPanel(t)
	p.w = 30
	p.prefs.Look = lookApp
	p.dispW = 80
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Name: "Api work", Status: status.Done, Created: 1},
		{ID: "b1", Dir: "/p/site", Name: "Site fix", Status: status.Working, Created: 2},
	}
	p.rebuildItems("")
	line := p.tabsLine("b1")
	if !strings.Contains(line, "#[range=user|a1]") || !strings.Contains(line, "#[range=user|b1]") {
		t.Fatalf("ranges: %q", line)
	}
	plain := stripTmux(line)
	if !strings.HasPrefix(plain, strings.Repeat(" ", 31)) || !strings.Contains(plain, "Api work") {
		t.Fatalf("tabs: %q", plain)
	}
	p.dispW = 20 // only the shown tab fits
	plain = stripTmux(p.tabsLine("b1"))
	if strings.Contains(plain, "Api work") || !strings.Contains(plain, "Site fix") || !strings.Contains(plain, "‹") {
		t.Fatalf("narrow tabs: %q", plain)
	}
	p.rebuildItems("a:a1")
	p.clickTab("b1")
	if p.selKey() != "a:b1" {
		t.Fatalf("click: %s", p.selKey())
	}
}

func TestRail(t *testing.T) {
	p := testPanel(t)
	p.w, p.h = railW, 20
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Name: "Api work", Status: status.Done, Created: 1},
		{ID: "b1", Dir: "/p/site", Name: "Site fix", Status: status.Waiting, Created: 2},
	}
	p.rebuildItems("")
	p.handleKey("b")
	if !p.rail {
		t.Fatal("b folds the list")
	}
	screen := stripANSI(strings.Join(p.frame(), "\n"))
	if strings.Count(screen, "✳") != 2 || strings.Contains(screen, "Api work") {
		t.Fatalf("rail:\n%s", screen)
	}
	p.handleKey("n") // the folder picker needs the full list
	if p.rail {
		t.Fatal("a picker opens the list again")
	}
}

func TestAppLookIsDefault(t *testing.T) {
	p := testPanel(t)
	p.prefs.Look = ""
	if !p.appLook() {
		t.Fatal("no choice saved: the app look")
	}
	p.handleKey("L")
	if p.appLook() || p.prefs.Look != lookClassic {
		t.Fatalf("L: %q", p.prefs.Look)
	}
}

func TestRailSurvivesLayoutChanges(t *testing.T) {
	p := testPanel(t)
	p.prefs.PanelWidth = 40
	p.rail = true
	p.w = railW
	p.noteWidth()
	if !p.rail {
		t.Fatal("a narrow strip must stay folded")
	}
	p.w = 50
	p.noteWidth()
	if p.rail {
		t.Fatal("a widened strip should open the full list")
	}
}
