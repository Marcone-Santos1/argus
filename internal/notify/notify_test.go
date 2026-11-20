package notify

import (
	"strings"
	"testing"
)

// The title and body go to the script as arguments, never inside it: a title
// with quotes cannot inject AppleScript.
func TestDarwinCommandKeepsTextOutOfScript(t *testing.T) {
	title := `x" & (do shell script "touch /tmp/pwn") & "`
	cmd := commandFor("darwin", title, "body")
	args := cmd.Args
	if len(args) != 5 || args[3] != title || args[4] != "body" {
		t.Fatalf("args: %q", args)
	}
	if strings.Contains(args[2], "touch") {
		t.Fatal("the text must not enter the script")
	}
}

// On Linux the text goes after "--", so it can never become an option.
func TestLinuxCommand(t *testing.T) {
	cmd := commandFor("linux", "--urgency=critical", "body")
	if cmd == nil {
		t.Skip("notify-send not installed")
	}
	args := cmd.Args
	if args[len(args)-3] != "--" || args[len(args)-2] != "--urgency=critical" {
		t.Fatalf("args: %q", args)
	}
}
