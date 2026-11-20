package notify

import (
	"strings"
	"testing"
)

// Título e texto vão como argumentos do script, nunca dentro dele: um título
// com aspas não consegue injetar AppleScript.
func TestCommandKeepsTextOutOfScript(t *testing.T) {
	title := `x" & (do shell script "touch /tmp/pwn") & "`
	cmd := Command(title, "corpo")
	args := cmd.Args
	if len(args) != 5 || args[3] != title || args[4] != "corpo" {
		t.Fatalf("args: %q", args)
	}
	if strings.Contains(args[2], "touch") {
		t.Fatal("o texto não pode entrar no script")
	}
}
