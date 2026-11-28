package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c, err := Parse(DefaultTOML, "/Users/ana")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Roots; !reflect.DeepEqual(got, []string{"/Users/ana/Projects", "/Users/ana/Sites"}) {
		t.Errorf("roots with ~ expanded: %v", got)
	}
	if len(c.Profiles) != 2 || c.Profiles[0].Name != "claude" || c.Profiles[1].Label() != "opencode" {
		t.Fatalf("profiles: %+v", c.Profiles)
	}
	if !c.ShouldNotify("waiting") || !c.ShouldNotify("done") || c.ShouldNotify("working") {
		t.Errorf("default notifications: %v", c.Notify)
	}
}

func TestClaudeDirs(t *testing.T) {
	c, err := Parse(`
[[profile]]
name = "work"
command = "claude"
env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }
[[profile]]
name = "personal"
command = "claude"
`, "/Users/ana")
	if err != nil {
		t.Fatal(err)
	}
	work, _ := c.Profile("work")
	if got := work.Env["CLAUDE_CONFIG_DIR"]; got != "/Users/ana/.claude-work" {
		t.Errorf("env with ~ expanded: %q", got)
	}
	if got := work.ClaudeStateFile("/Users/ana"); got != "/Users/ana/.claude-work/.claude.json" {
		t.Errorf("work ClaudeStateFile: %q", got)
	}
	personal, _ := c.Profile("personal")
	if got := personal.ClaudeStateFile("/Users/ana"); got != "/Users/ana/.claude.json" {
		t.Errorf("default ClaudeStateFile: %q", got)
	}
	if got := personal.ClaudeConfigDir("/Users/ana"); got != "/Users/ana/.claude" {
		t.Errorf("default ClaudeConfigDir: %q", got)
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse(`notify = []`, "/h")
	if err != nil {
		t.Fatal(err)
	}
	if c.ShouldNotify("waiting") {
		t.Error("notify = [] must turn notifications off")
	}
	if c.Depth != 2 || c.MaxSessions != 8 || len(c.Profiles) != 1 {
		t.Errorf("defaults: %+v", c)
	}
	if c.Profiles[0].Group != "PERSONAL" {
		t.Errorf("default profile group: %q", c.Profiles[0].Group)
	}
	if _, err := Parse(`roots = [`, "/h"); err == nil {
		t.Error("invalid TOML must fail")
	}
}

func TestLabelAndTag(t *testing.T) {
	cases := []struct {
		p          Profile
		label, tag string
	}{
		{Profile{Name: "a", Command: "claude --x", Group: "PERSONAL"}, "claude", "per"},
		{Profile{Name: "b", Command: "/opt/bin/opencode", Group: "PERSONAL"}, "opencode", "oc"},
		{Profile{Name: "c", Command: "claude", Group: "WORK"}, "claude", "wor"},
		{Profile{Name: "d", Command: "", Group: "UI"}, "d", "ui"},
		{Profile{Name: "e", Command: "gemini", Group: "X"}, "gemini", "gem"},
		{Profile{Name: "f", Command: "claude", Group: "EMPRESA", TagText: "job"}, "claude", "job"},
	}
	for _, tc := range cases {
		if got := tc.p.Label(); got != tc.label {
			t.Errorf("Label(%q) = %q", tc.p.Command, got)
		}
		if got := tc.p.Tag(); got != tc.tag {
			t.Errorf("Tag(%+v) = %q", tc.p, got)
		}
	}
}

func TestDefaultProfile(t *testing.T) {
	c, err := Parse(`
[[profile]]
name = "personal"
command = "claude"
[[profile]]
name = "work"
command = "claude"
[[profile]]
name = "opencode"
command = "opencode"
[[rule]]
match = "/work-"
profile = "work"
`, "/h")
	if err != nil {
		t.Fatal(err)
	}
	prefs := &Prefs{Dirs: map[string]DirPref{"/h/p/remembered": {Profile: "opencode"}, "/h/p/gone": {Profile: "deleted"}}}
	cases := map[string]string{
		"/h/p/work-api":   "work",     // rule
		"/h/p/remembered": "opencode", // remembered
		"/h/p/gone":       "personal", // remembered, but the profile was removed
		"/h/p/other":      "personal", // first profile
	}
	for dir, want := range cases {
		if got := c.DefaultProfile(dir, prefs); got != want {
			t.Errorf("DefaultProfile(%s) = %s, want %s", dir, got, want)
		}
	}
	if got := c.DefaultProfile("/x", nil); got != "personal" {
		t.Errorf("without prefs: %s", got)
	}
}

func TestPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	p := LoadPrefs(path)
	if len(p.Dirs) != 0 {
		t.Fatal("a missing file gives empty prefs")
	}
	p.Dirs["/a"] = DirPref{Profile: "x", Last: 42}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if got := LoadPrefs(path).Dirs["/a"]; got != (DirPref{Profile: "x", Last: 42}) {
		t.Fatalf("read back: %+v", got)
	}
}

func TestProfilesFor(t *testing.T) {
	c, _ := Parse(DefaultTOML, "/h")
	if n := len(c.ProfilesFor("claude")); n != 1 {
		t.Errorf("claude profiles: %d", n)
	}
	if n := len(c.ProfilesFor("opencode")); n != 1 {
		t.Errorf("opencode profiles: %d", n)
	}
}

func TestDisplay(t *testing.T) {
	c, err := Parse(`
[[profile]]
name = "a"
group = "WORK"
command = "claude"

[[profile]]
name = "b"
group = "WORK"
command = "claude"
display = "Claude da empresa"
`, "/h")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Profiles[0].Display(); got != "claude wor" {
		t.Errorf("padrão: %q", got)
	}
	if got := c.Profiles[1].Display(); got != "Claude da empresa" {
		t.Errorf("display do config: %q", got)
	}
}

func TestTemplates(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfgPath, []byte(`
[[profile]]
name = "claude"
command = "claude"
[[template]]
name = "review"
dir = "~/p/api"
profile = "claude"
prompt = "Read the notes"
`), 0o644)
	c, err := Load(cfgPath, "/home/ana")
	if err != nil || len(c.Templates) != 1 || c.Templates[0].Dir != "/home/ana/p/api" || c.Templates[0].Saved {
		t.Fatalf("config template: %+v %v", c.Templates, err)
	}
	saved := append(c.Templates, Template{Name: "it's \"new\"", Dir: "/x", Profile: "claude", Prompt: "multi\nline", Saved: true})
	if err := SaveTemplates(filepath.Join(dir, TemplatesFile), saved); err != nil {
		t.Fatal(err)
	}
	c, _ = Load(cfgPath, "/home/ana")
	if len(c.Templates) != 2 || !c.Templates[1].Saved || c.Templates[1].Name != `it's "new"` || c.Templates[1].Prompt != "multi\nline" {
		t.Fatalf("saved template round trip: %+v", c.Templates)
	}
}
