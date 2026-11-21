package ui

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/history"
)

func TestParseMouse(t *testing.T) {
	got := parseKeys([]byte("\x1b[<0;12;5M\x1b[<0;12;5m\x1b[<64;3;1M\x1b[<65;3;1M\x1b[<2;3;1Mj"))
	want := []string{"mouse:click:4", "mouse:wheelup", "mouse:wheeldown", "j"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKeys: %q, want %q", got, want)
	}
	if got := parseKeys([]byte("\x1b[C\x1b[D")); !reflect.DeepEqual(got, []string{"right", "left"}) {
		t.Fatalf("arrows: %q", got)
	}
}

func TestClickTheList(t *testing.T) {
	p := testPanel(t)
	p.w, p.h = 60, 30
	now := time.Now()
	p.histAll = []history.Entry{
		{Tool: "claude", ID: "c1", Dir: "/p/api", Title: "One", Updated: now},
		{Tool: "claude", ID: "c2", Dir: "/p/api", Title: "Two", Updated: now},
		{Tool: "claude", ID: "c3", Dir: "/p/docs", Title: "Docs", Updated: now},
	}
	p.rebuildItems("")
	p.viewList()
	row := func(key string) int {
		for r, t := range p.rowTarget {
			if t >= 0 && p.itemKey(t) == key {
				return r
			}
		}
		t.Fatalf("no line for %s: %v", key, p.rowTarget)
		return -1
	}
	click := func(r int) {
		p.handleKey("mouse:click:" + itoa(r))
		p.viewList()
	}
	// A click on a folder opens it; its conversations get lines too.
	click(row("f:/p/api"))
	if !p.expanded["/p/api"] || p.selKey() != "f:/p/api" {
		t.Fatalf("click opens the folder: %v %s", p.expanded, p.selKey())
	}
	// One click on a conversation only selects it.
	p.lastClick = time.Time{}
	click(row("r:claude:c2"))
	if p.selKey() != "r:claude:c2" || p.mode != modeList {
		t.Fatalf("click selects: %s", p.selKey())
	}
	// A click on an empty line or the footer does nothing.
	p.lastClick = time.Time{}
	click(p.h - 1)
	if p.selKey() != "r:claude:c2" {
		t.Fatal("a click on the footer changed the selection")
	}
	// The wheel moves the selection.
	p.handleKey("mouse:wheelup")
	if p.selKey() != "r:claude:c1" {
		t.Fatalf("wheel: %s", p.selKey())
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
