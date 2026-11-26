package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/usage"
)

func TestShouldCompact(t *testing.T) {
	p := testPanel(t)
	p.d.Config.CompactAt = 70
	now := time.Now()
	a := agent{ID: "a1", Profile: "personal", Status: status.Done}
	p.usage["a1"] = usage.Info{Context: 82, At: now}
	if !p.shouldCompact(a) {
		t.Fatal("done above the threshold: compact")
	}
	p.compacted["a1"] = now.Add(time.Second)
	if p.shouldCompact(a) {
		t.Fatal("no new reading since the last compact: wait")
	}
	delete(p.compacted, "a1")
	for name, b := range map[string]agent{
		"working":  {ID: "a1", Profile: "personal", Status: status.Working},
		"asleep":   {ID: "a1", Profile: "personal", Status: status.Done, Asleep: now},
		"opencode": {ID: "a1", Profile: "opencode", Status: status.Done},
	} {
		if p.shouldCompact(b) {
			t.Errorf("%s: no compact", name)
		}
	}
	p.usage["a1"] = usage.Info{Context: 60, At: now}
	if p.shouldCompact(a) {
		t.Fatal("below the threshold")
	}
	p.d.Config.CompactAt = 0
	p.usage["a1"] = usage.Info{Context: 95, At: now}
	if p.shouldCompact(a) {
		t.Fatal("off by default")
	}
}

// C: the session gets the handoff text; when its turn ends a new
// conversation starts in the same pane with the resume text.
func TestHandoffWithRealTmux(t *testing.T) {
	needTmux(t)
	home := t.TempDir()
	sock := fmt.Sprintf("argus-handoff-%d", os.Getpid())
	p := paths.For(home, sock)
	_ = os.MkdirAll(p.State, 0o755)
	logf := filepath.Join(t.TempDir(), "log")
	bin := filepath.Join(t.TempDir(), "claude") // the label decides it is Claude
	_ = os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'args:%s\\n' \"$*\" >> "+logf+"\nwhile read -r l; do printf 'typed:%s\\n' \"$l\" >> "+logf+"; done\n"), 0o755)
	cfg, _ := config.Parse("[[profile]]\nname = \"fake\"\ncommand = \""+bin+"\"\n", home)
	pn, tc := newServer(t, sock, p, cfg)
	t.Cleanup(func() { _, _ = tc.Run("kill-server") })
	pn.createAgent(t.TempDir(), cfg.Profiles[0], nil, "")
	time.Sleep(300 * time.Millisecond)
	pn.agents[0].Status = status.Done
	id, pane := pn.agents[0].ID, pn.agents[0].Pane

	pn.handoffSelected()
	if pn.mode != modeConfirm {
		t.Fatalf("C asks first (msg %q)", pn.msg)
	}
	fn := pn.confirmFn
	pn.mode, pn.confirmFn = modeList, nil
	fn()
	time.Sleep(800 * time.Millisecond)
	file := pn.handoffs[id]
	if b, _ := os.ReadFile(logf); !strings.Contains(string(b), "typed:Write a handoff to "+file) {
		t.Fatalf("handoff text typed into the session:\n%s", b)
	}

	// The turn ends without the file: nothing restarts.
	pn.finishHandoff(id)
	if !strings.Contains(pn.msg, "handoff") {
		t.Fatalf("missing file reported: %q", pn.msg)
	}
	pn.handoffs[id] = file
	_ = os.WriteFile(file, []byte("goal: x\n"), 0o644)
	pn.finishHandoff(id)
	time.Sleep(500 * time.Millisecond)
	b, _ := os.ReadFile(logf)
	if !strings.Contains(string(b), "args:Continue the work described in "+file) {
		t.Fatalf("new conversation with the resume text:\n%s", b)
	}
	if len(pn.agents) != 1 || pn.agents[0].ID != id || pn.agents[0].Pane != pane {
		t.Fatalf("same session, same pane: %+v", pn.agents)
	}
	if _, ok := pn.handoffs[id]; ok {
		t.Fatal("handoff done")
	}
}
