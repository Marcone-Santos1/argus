package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The generated tmux.conf must load in a real tmux, with every typing key bound.
func TestTmuxConfLoads(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	conf := filepath.Join(t.TempDir(), "tmux.conf")
	if err := os.WriteFile(conf, []byte(TmuxConf(t.TempDir())), 0o644); err != nil {
		t.Fatal(err)
	}
	sock := fmt.Sprintf("argus-conf-%d", os.Getpid())
	run := func(args ...string) (string, error) {
		out, err := exec.Command("tmux", append([]string{"-L", sock}, args...)...).CombinedOutput()
		return string(out), err
	}
	t.Cleanup(func() { _, _ = run("kill-server") })
	if out, err := run("-f", "/dev/null", "new-session", "-d", "sleep", "30"); err != nil {
		t.Fatal(out)
	}
	if out, err := run("source-file", conf); err != nil {
		t.Fatalf("tmux.conf does not load: %s", out)
	}
	keys, _ := run("list-keys", "-T", "copy-mode")
	for _, want := range []string{"MouseDragEnd1Pane", "copy-pipe-no-clear", "send-keys -l"} {
		if !strings.Contains(keys, want) {
			t.Errorf("copy-mode missing %q", want)
		}
	}
	n := strings.Count(keys, `send-keys -X cancel \; send-keys`)
	if n < 94+4 {
		t.Errorf("typing keys bound in copy-mode: %d", n)
	}
}
