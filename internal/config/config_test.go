package config

import (
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
		t.Errorf("roots com ~ expandido: %v", got)
	}
	if len(c.Profiles) != 3 || c.Profiles[0].Name != "claude-pessoal" {
		t.Fatalf("perfis: %+v", c.Profiles)
	}
	work, _ := c.Profile("claude-work")
	if got := work.Env["CLAUDE_CONFIG_DIR"]; got != "/Users/ana/.claude-work" {
		t.Errorf("env com ~ expandido: %q", got)
	}
	if got := work.ClaudeStateFile("/Users/ana"); got != "/Users/ana/.claude-work/.claude.json" {
		t.Errorf("ClaudeStateFile do work: %q", got)
	}
	pes, _ := c.Profile("claude-pessoal")
	if got := pes.ClaudeStateFile("/Users/ana"); got != "/Users/ana/.claude.json" {
		t.Errorf("ClaudeStateFile padrão: %q", got)
	}
	if !c.ShouldNotify("waiting") || !c.ShouldNotify("done") || c.ShouldNotify("working") {
		t.Errorf("avisos padrão: %v", c.Notify)
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse(`notify = []`, "/h")
	if err != nil {
		t.Fatal(err)
	}
	if c.ShouldNotify("waiting") {
		t.Error("notify = [] deve desligar os avisos")
	}
	if c.Depth != 2 || c.MaxSessions != 8 || len(c.Profiles) != 1 {
		t.Errorf("padrões: %+v", c)
	}
	if c.Profiles[0].Group != "PESSOAL" {
		t.Errorf("grupo do perfil padrão: %q", c.Profiles[0].Group)
	}
	if _, err := Parse(`roots = [`, "/h"); err == nil {
		t.Error("TOML inválido deveria dar erro")
	}
}

func TestLabelAndTag(t *testing.T) {
	cases := []struct {
		p          Profile
		label, tag string
	}{
		{Profile{Name: "a", Command: "claude --x", Group: "PESSOAL"}, "claude", "pes"},
		{Profile{Name: "b", Command: "/opt/bin/opencode", Group: "PESSOAL"}, "opencode", "oc"},
		{Profile{Name: "c", Command: "claude", Group: "EMPRESA"}, "claude", "emp"},
		{Profile{Name: "d", Command: "", Group: "UI"}, "d", "ui"},
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
	c, err := Parse(DefaultTOML+"\n[[rule]]\nmatch = \"/work-\"\nprofile = \"claude-work\"\n", "/h")
	if err != nil {
		t.Fatal(err)
	}
	prefs := &Prefs{Dirs: map[string]DirPref{"/h/p/lembrado": {Profile: "opencode"}, "/h/p/sumiu": {Profile: "nao-existe"}}}
	cases := map[string]string{
		"/h/p/work-api": "claude-work",    // regra
		"/h/p/lembrado": "opencode",       // lembrado
		"/h/p/sumiu":    "claude-pessoal", // lembrado, mas perfil apagado
		"/h/p/qualquer": "claude-pessoal", // primeiro perfil
	}
	for dir, want := range cases {
		if got := c.DefaultProfile(dir, prefs); got != want {
			t.Errorf("DefaultProfile(%s) = %s, quer %s", dir, got, want)
		}
	}
	if got := c.DefaultProfile("/x", nil); got != "claude-pessoal" {
		t.Errorf("sem prefs: %s", got)
	}
}

func TestPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	p := LoadPrefs(path)
	if len(p.Dirs) != 0 {
		t.Fatal("arquivo ausente deveria dar prefs vazias")
	}
	p.Dirs["/a"] = DirPref{Profile: "x", Last: 42}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if got := LoadPrefs(path).Dirs["/a"]; got != (DirPref{Profile: "x", Last: 42}) {
		t.Fatalf("relido: %+v", got)
	}
}

func TestProfilesFor(t *testing.T) {
	c, _ := Parse(DefaultTOML, "/h")
	if n := len(c.ProfilesFor("claude")); n != 2 {
		t.Errorf("perfis claude: %d", n)
	}
	if n := len(c.ProfilesFor("opencode")); n != 1 {
		t.Errorf("perfis opencode: %d", n)
	}
}
