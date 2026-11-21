package ui

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A real Claude Code permission prompt, captured from tmux.
func TestParseClaudePermission(t *testing.T) {
	pr, ok := parsePrompt(fixture(t, "claude-permission.txt"))
	if !ok {
		t.Fatal("prompt not found")
	}
	if !pr.Numbered || pr.Selected != 0 || len(pr.Options) != 4 {
		t.Fatalf("options: %+v", pr)
	}
	if pr.Options[0] != "Yes" || pr.Options[3] != "No" {
		t.Errorf("first/last option: %q / %q", pr.Options[0], pr.Options[3])
	}
	if !strings.HasPrefix(pr.Options[1], "Yes, and don't ask again") || !strings.HasSuffix(pr.Options[1], "/tmp/lab") {
		t.Errorf("an option's continuation line joins it: %q", pr.Options[1])
	}
	if pr.Question != "Do you want to proceed?" {
		t.Errorf("question: %q", pr.Question)
	}
	if pr.Context != "touch outro-arquivo.txt && rtk ls -l outro-arquivo.txt" {
		t.Errorf("context is the command: %q", pr.Context)
	}
}

// The folder trust dialog: options without numbers, cursor on the first.
func TestParseUnnumbered(t *testing.T) {
	pr, ok := parsePrompt(fixture(t, "claude-trust.txt"))
	if !ok {
		t.Fatal("prompt not found")
	}
	if pr.Numbered || pr.Selected != 0 || !reflect.DeepEqual(pr.Options, []string{"No, exit", "Yes, I trust this folder"}) {
		t.Fatalf("prompt: %+v", pr)
	}
}

func TestParseNoPrompt(t *testing.T) {
	screens := []string{
		"",
		"⏺ Done.\n❯ \n",
		// A numbered list in the agent's answer, followed by the input box: not a prompt.
		"⏺ Steps:\n  1. build\n  2. test\n  3. ship\n\n✻ done\n────\n❯ \n────\n  ⏵⏵ auto mode on\n",
	}
	for _, s := range screens {
		if pr, ok := parsePrompt(s); ok {
			t.Errorf("no prompt expected in %q, got %+v", s, pr)
		}
	}
}

func TestShort(t *testing.T) {
	if got := short("Yes, and don't ask again", 10); got != "Yes, and…" {
		t.Errorf("short: %q", got)
	}
	if got := short("No", 10); got != "No" {
		t.Errorf("short keeps short text: %q", got)
	}
}
