package procs

import "testing"

func TestShells(t *testing.T) {
	all := []Proc{
		{1, 0, "tmux"},
		{10, 1, "claude"},
		{11, 10, "node"}, // an MCP server
		{12, 10, "zsh"},  // a background command
		{13, 12, "docker"},
		{14, 12, "zsh"},  // inside it: not another one
		{15, 10, "bash"}, // a second background command
		{20, 1, "sh"},    // a session started through sh -c
		{21, 20, "claude"},
		{22, 21, "zsh"},
	}
	if n := Shells(all, 10); n != 2 {
		t.Fatalf("claude: %d shells, want 2", n)
	}
	if n := Shells(all, 20); n != 1 {
		t.Fatalf("sh -c claude: %d shells, want 1", n)
	}
	if n := Shells(all, 11); n != 0 {
		t.Fatalf("MCP server: %d", n)
	}
}

func TestCountReadsTheTable(t *testing.T) {
	if _, ok := Count([]int{1}); !ok {
		t.Skip("no process table here")
	}
}
