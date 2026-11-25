package ui

import (
	"github.com/Marcone-Santos1/argus/internal/install"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

func TestFollowClaudeTheme(t *testing.T) {
	home := t.TempDir()
	cfg, err := config.Parse(`
[[profile]]
name = "pes"
command = "claude"
[[profile]]
name = "emp"
command = "claude"
env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }
`, home)
	if err != nil {
		t.Fatal(err)
	}
	pes, emp := filepath.Join(home, ".claude", "settings.json"), filepath.Join(home, ".claude-work", "settings.json")
	for _, f := range []string{pes, emp} {
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		_ = os.WriteFile(f, []byte(`{"theme":"dark","other":1}`), 0o600)
	}
	_ = os.MkdirAll(filepath.Join(home, ".claude", "themes"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".claude", "themes", "argus-light.json"), []byte(`{"base":"light"}`), 0o644)
	p := New(Deps{Paths: paths.For(home, "x"), Config: cfg, Tmux: tmux.New("argus-theme-test-missing")})
	t.Cleanup(func() { setPalette(install.ThemeDark) })

	p.themeCheck(true)
	if lightLook {
		t.Fatal("dark at start")
	}
	// Picking Argus light in one account: the look turns light, the other
	// account follows, and the editor gets the light theme.
	later := time.Now().Add(2 * time.Second)
	_ = os.WriteFile(pes, []byte(`{"theme":"custom:argus-light","other":1}`), 0o600)
	_ = os.Chtimes(pes, later, later)
	p.themeCheck(true)
	if !lightLook {
		t.Fatal("light after the pick")
	}
	data, _ := os.ReadFile(emp)
	if gjson.GetBytes(data, "theme").String() != "custom:argus" || gjson.GetBytes(data, "other").Int() != 1 {
		t.Fatalf("other account: %s", data)
	}
	if fi, _ := os.Stat(emp); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode kept: %v", fi.Mode())
	}
	fresh, _ := os.ReadFile(filepath.Join(home, ".config", "fresh", "config.json"))
	if gjson.GetBytes(fresh, "theme").String() != "argus-light" {
		t.Fatalf("fresh: %s", fresh)
	}
	// An earlier version's fixed theme became the one Argus theme, light.
	argus := func(dir string) string {
		data, _ := os.ReadFile(filepath.Join(home, dir, "themes", "argus.json"))
		return gjson.GetBytes(data, "base").String()
	}
	if argus(".claude") != "light" || argus(".claude-work") != "light" {
		t.Fatalf("theme files: %q %q", argus(".claude"), argus(".claude-work"))
	}
	// T turns it dark: the theme file changes (Claude reloads it live), the
	// accounts stay on it.
	p.handleKey("T")
	p.handleKey("1")
	for _, f := range []string{pes, emp} {
		data, _ := os.ReadFile(f)
		if gjson.GetBytes(data, "theme").String() != "custom:argus" {
			t.Fatalf("T: %s", data)
		}
	}
	if lightLook || argus(".claude") != "dark" || argus(".claude-work") != "dark" {
		t.Fatalf("dark after T: %v %q", lightLook, argus(".claude"))
	}
	// Then night, purple on blue-gray, in Argus, Claude and the editor.
	p.handleKey("T")
	p.handleKey("down") // a preview, in Argus only
	if paletteMode != install.ThemeNight || argus(".claude") != "dark" {
		t.Fatalf("preview: %q %q", paletteMode, argus(".claude"))
	}
	p.handleKey("esc")
	if paletteMode != install.ThemeDark {
		t.Fatalf("esc goes back: %q", paletteMode)
	}
	p.handleKey("T")
	p.handleKey("down")
	p.handleKey("enter")
	fresh, _ = os.ReadFile(filepath.Join(home, ".config", "fresh", "config.json"))
	if paletteMode != install.ThemeNight || argus(".claude") != "dark" || gjson.GetBytes(fresh, "theme").String() != "argus-night" {
		t.Fatalf("night after T: %q %s", paletteMode, fresh)
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".claude", "themes", "argus.json")); gjson.GetBytes(data, "overrides.claude").String() != "#ff79c6" {
		t.Fatalf("night theme file: %s", data)
	}
	if live, _ := os.ReadFile(filepath.Join(home, ".config", "fresh", "argus-theme")); string(live) != "argus-night\n" {
		t.Fatalf("open editors: %q", live)
	}
	p.handleKey("T")
	p.handleKey("7")
	if !lightLook {
		t.Fatal("light after night")
	}
	// The classic look stays dark whatever the theme.
	p.handleKey("L")
	if lightLook {
		t.Fatal("classic is dark")
	}
}
