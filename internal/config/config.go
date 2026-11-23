// Package config reads the Argus configuration (profiles, project folders)
// and the preferences remembered per folder.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Profile is a command plus its environment (a Claude account, opencode...).
type Profile struct {
	Name    string            `toml:"name"`
	Group   string            `toml:"group"`
	Command string            `toml:"command"`
	Env     map[string]string `toml:"env"`
	TagText string            `toml:"tag"`     // optional short label; derived from the group when empty
	Show    string            `toml:"display"` // optional list text; "<program> <tag>" when empty
}

// Display is the text the list shows for the profile's sessions, e.g.
// "claude per", or whatever "display" sets in config.toml.
func (p Profile) Display() string {
	if p.Show != "" {
		return p.Show
	}
	return p.Label() + " " + p.Tag()
}

// Label is the program's short name (claude, opencode, gemini...).
func (p Profile) Label() string {
	f := strings.Fields(p.Command)
	if len(f) == 0 {
		return p.Name
	}
	return filepath.Base(f[0])
}

// ClaudeConfigDir is the Claude config directory the profile uses.
func (p Profile) ClaudeConfigDir(home string) string {
	if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
		return v
	}
	return filepath.Join(home, ".claude")
}

// ClaudeStateFile is the profile's .claude.json (holds the logged-in account).
func (p Profile) ClaudeStateFile(home string) string {
	if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
		return filepath.Join(v, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

// Tag is the short account label shown in the list: per, wor, oc...
func (p Profile) Tag() string {
	if p.TagText != "" {
		return p.TagText
	}
	switch p.Label() {
	case "opencode":
		return "oc"
	case "gemini":
		return "gem"
	case "codex":
		return "cdx"
	}
	g := []rune(strings.ToLower(p.Group))
	if len(g) > 3 {
		g = g[:3]
	}
	return string(g)
}

// Rule picks a profile for folders that contain Match.
type Rule struct {
	Match   string `toml:"match"`
	Profile string `toml:"profile"`
}

type Config struct {
	Roots       []string `toml:"roots"`
	Depth       int      `toml:"depth"`
	MaxSessions int      `toml:"max_sessions"`
	// HibernateAfter closes a finished session's process after this long
	// without use ("2h"); it resumes on Enter. "0" turns it off.
	HibernateAfter string     `toml:"hibernate_after"`
	Notify         []string   `toml:"notify"`
	Profiles       []Profile  `toml:"profile"`
	Rules          []Rule     `toml:"rule"`
	Templates      []Template `toml:"template"`
}

// Template is a session ready to start with one key (t): a folder, a
// profile and an optional first message.
type Template struct {
	Name    string `toml:"name"`
	Dir     string `toml:"dir"`
	Profile string `toml:"profile"`
	Prompt  string `toml:"prompt,omitempty"`
	Saved   bool   `toml:"-"` // created from the panel (templates.toml), so it can be deleted there
}

// DefaultTOML is the config.toml written by install.
const DefaultTOML = `# Argus configuration. This file is yours; install never overwrites it.

# Folders where "n" looks for projects, and how many levels deep.
roots = ["~/Projects", "~/Sites"]
depth = 2

# Above this, starting a session asks for confirmation.
max_sessions = 8

# A finished session untouched for this long is hibernated: its process is
# closed (freeing its memory) and it stays in the list; Enter resumes the
# conversation. "0" turns it off.
hibernate_after = "2h"

# macOS/desktop notifications when a session changes to these states
# (waiting = waiting for you, done = finished). An empty list turns them off.
notify = ["waiting", "done"]

# Profiles: a command plus its environment. The first one is the default.
# "group" is shown as a short tag in the list (its first three letters),
# or set "tag" explicitly. "display" replaces the whole list text, which is
# "<program> <tag>" by default (for example display = "Claude personal").
[[profile]]
name = "claude"
group = "PERSONAL"
command = "claude"

# A second Claude account, with its own config directory:
# [[profile]]
# name = "claude-work"
# group = "WORK"
# command = "claude"
# env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }

[[profile]]
name = "opencode"
group = "PERSONAL"
command = "opencode"

# [[profile]]
# name = "gemini"
# group = "PERSONAL"
# command = "gemini"

# [[profile]]
# name = "codex"
# group = "PERSONAL"
# command = "codex"

# Templates: t starts one (folder, profile and an optional first message).
# The panel also saves the ones you create there in templates.toml.
# [[template]]
# name = "api review"
# dir = "~/Projects/api"
# profile = "claude-work"
# prompt = "Read NOTES.md and tell me what is next."

# Optional rules: a folder containing the text opens with the profile.
# [[rule]]
# match = "/work-"
# profile = "claude-work"
`

// Load reads config.toml (or the default when it does not exist) and expands
// "~". The templates saved from the panel (templates.toml, next to it) are
// added after the ones config.toml declares.
func Load(path, home string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte(DefaultTOML)
	}
	c, err := Parse(string(data), home)
	if err != nil {
		return nil, err
	}
	c.Templates = append(c.Templates, LoadTemplates(filepath.Join(filepath.Dir(path), TemplatesFile), home)...)
	return c, nil
}

// TemplatesFile holds the templates created from the panel.
const TemplatesFile = "templates.toml"

// LoadTemplates reads templates.toml (nil when missing or invalid).
func LoadTemplates(path, home string) []Template {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Templates []Template `toml:"template"`
	}
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil
	}
	for i := range f.Templates {
		f.Templates[i].Dir = expandHome(f.Templates[i].Dir, home)
		f.Templates[i].Saved = true
	}
	return f.Templates
}

// SaveTemplates rewrites templates.toml with the panel's templates.
func SaveTemplates(path string, ts []Template) error {
	var b strings.Builder
	b.WriteString("# Templates created in the Argus panel (t). Edit or delete freely.\n")
	enc := toml.NewEncoder(&b)
	var f struct {
		Templates []Template `toml:"template"`
	}
	for _, t := range ts {
		if t.Saved {
			f.Templates = append(f.Templates, t)
		}
	}
	if err := enc.Encode(f); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func expandHome(s, home string) string {
	if s == "~" {
		return home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}

// Parse interprets a config.toml.
func Parse(data, home string) (*Config, error) {
	var c Config
	md, err := toml.Decode(data, &c)
	if err != nil {
		return nil, err
	}
	expand := func(s string) string {
		if s == "~" {
			return home
		}
		if strings.HasPrefix(s, "~/") {
			return filepath.Join(home, s[2:])
		}
		return s
	}
	if len(c.Roots) == 0 {
		c.Roots = []string{"~/Projects"}
	}
	for i := range c.Roots {
		c.Roots[i] = expand(c.Roots[i])
	}
	if c.Depth < 1 {
		c.Depth = 2
	}
	if c.MaxSessions < 1 {
		c.MaxSessions = 8
	}
	if !md.IsDefined("hibernate_after") {
		c.HibernateAfter = "2h"
	}
	if !md.IsDefined("notify") {
		c.Notify = []string{"waiting", "done"}
	}
	if len(c.Profiles) == 0 {
		c.Profiles = []Profile{{Name: "claude", Group: "PERSONAL", Command: "claude"}}
	}
	for i := range c.Templates {
		c.Templates[i].Dir = expand(c.Templates[i].Dir)
	}
	for i := range c.Profiles {
		if c.Profiles[i].Group == "" {
			c.Profiles[i].Group = "OTHER"
		}
		for k, v := range c.Profiles[i].Env {
			c.Profiles[i].Env[k] = expand(v)
		}
	}
	return &c, nil
}

// Profile finds a profile by name.
func (c *Config) Profile(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// ProfilesFor lists the profiles of one program (claude, opencode, gemini...).
func (c *Config) ProfilesFor(label string) []Profile {
	var ps []Profile
	for _, p := range c.Profiles {
		if p.Label() == label {
			ps = append(ps, p)
		}
	}
	return ps
}

// ShouldNotify reports whether a change to state s triggers a notification.
func (c *Config) ShouldNotify(s string) bool {
	for _, n := range c.Notify {
		if n == s {
			return true
		}
	}
	return false
}

// DefaultProfile picks a folder's profile: a rule, then the last one used
// there, then the first profile.
func (c *Config) DefaultProfile(dir string, prefs *Prefs) string {
	for _, r := range c.Rules {
		if r.Match != "" && strings.Contains(dir, r.Match) {
			if _, ok := c.Profile(r.Profile); ok {
				return r.Profile
			}
		}
	}
	if prefs != nil {
		if dp, ok := prefs.Dirs[dir]; ok {
			if _, ok := c.Profile(dp.Profile); ok {
				return dp.Profile
			}
		}
	}
	return c.Profiles[0].Name
}

// DirPref is what Argus remembers about a folder.
type DirPref struct {
	Profile string `json:"profile"`
	Last    int64  `json:"last"`
}

// Prefs are the preferences remembered per folder.
type Prefs struct {
	Dirs map[string]DirPref `json:"dirs"`
	// Sessions maps a conversation ("tool:id") to the profile it was last
	// opened with. Claude does not record an account switch in the
	// transcript, so without this a switched conversation would reopen in
	// its original account.
	Sessions map[string]string `json:"sessions,omitempty"`
	// PanelWidth is the list's width in columns, as last dragged (0 = default).
	PanelWidth int `json:"panel_width,omitempty"`
	path       string
}

// LoadPrefs reads the preferences; a missing or invalid file yields empty ones.
func LoadPrefs(path string) *Prefs {
	p := &Prefs{Dirs: map[string]DirPref{}, path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, p)
	}
	if p.Dirs == nil {
		p.Dirs = map[string]DirPref{}
	}
	if p.Sessions == nil {
		p.Sessions = map[string]string{}
	}
	return p
}

// Save writes the preferences atomically.
func (p *Prefs) Save() error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// Hibernate is how long a finished session waits before being hibernated
// (0 = never).
func (c *Config) Hibernate() time.Duration {
	if c.HibernateAfter == "" || c.HibernateAfter == "0" {
		return 0
	}
	d, err := time.ParseDuration(c.HibernateAfter)
	if err != nil || d < 0 {
		return 0
	}
	return d
}
