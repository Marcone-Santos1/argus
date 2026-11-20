package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

var (
	stateDir = stateDirFor(os.Getenv("ARGUS_SOCKET"))
	shareDir = filepath.Join(homeDir(), ".local", "share", "argus")
	confDir  = filepath.Join(homeDir(), ".config", "argus")
)

// Um servidor de teste (ARGUS_SOCKET) usa outro diretório de estado, para não
// apagar o estado das sessões do servidor de verdade.
func stateDirFor(socket string) string {
	d := filepath.Join(homeDir(), ".local", "state", "argus")
	if socket != "" && socket != "argus" {
		d += "-" + socket
	}
	return d
}

func hookPath() string     { return filepath.Join(shareDir, "hook.sh") }
func tmuxConfPath() string { return filepath.Join(confDir, "tmux.conf") }
func configPath() string   { return filepath.Join(confDir, "config.toml") }
func prefsPath() string    { return filepath.Join(stateDir, "_prefs.json") }

func expand(p string) string {
	if p == "~" {
		return homeDir()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

type Profile struct {
	Name    string            `toml:"name"`
	Group   string            `toml:"group"`
	Command string            `toml:"command"`
	Env     map[string]string `toml:"env"`
}

// Label curto mostrado na lista (claude, opencode...).
func (p Profile) Label() string {
	f := strings.Fields(p.Command)
	if len(f) == 0 {
		return p.Name
	}
	return filepath.Base(f[0])
}

type Rule struct {
	Match   string `toml:"match"`
	Profile string `toml:"profile"`
}

type Config struct {
	Roots       []string  `toml:"roots"`
	Depth       int       `toml:"depth"`
	MaxSessions int       `toml:"max_sessions"`
	Profiles    []Profile `toml:"profile"`
	Rules       []Rule    `toml:"rule"`
}

const defaultConfig = `# Argus: configuração. Este arquivo é seu; o install não o sobrescreve.

# Pastas onde o "n" procura projetos, e quantos níveis descer.
roots = ["~/Projects", "~/Sites"]
depth = 2

# Acima disso, criar sessão pede confirmação.
max_sessions = 8

# Perfis: um comando com seu ambiente. O primeiro é o padrão.
[[profile]]
name = "claude-pessoal"
group = "PESSOAL"
command = "claude"

[[profile]]
name = "claude-work"
group = "EMPRESA"
command = "claude"
env = { CLAUDE_CONFIG_DIR = "~/.claude-work" }

[[profile]]
name = "opencode"
group = "PESSOAL"
command = "opencode"

# Regras opcionais: pasta que contém o trecho abre com o perfil.
# [[rule]]
# match = "/work-"
# profile = "claude-work"
`

func loadConfig() (*Config, error) {
	var c Config
	data, err := os.ReadFile(configPath())
	if err != nil {
		data = []byte(defaultConfig)
	}
	if _, err := toml.Decode(string(data), &c); err != nil {
		return nil, err
	}
	if len(c.Roots) == 0 {
		c.Roots = []string{"~/Projects"}
	}
	if c.Depth < 1 {
		c.Depth = 2
	}
	if c.MaxSessions < 1 {
		c.MaxSessions = 8
	}
	if len(c.Profiles) == 0 {
		c.Profiles = []Profile{{Name: "claude", Group: "PESSOAL", Command: "claude"}}
	}
	for i := range c.Profiles {
		if c.Profiles[i].Group == "" {
			c.Profiles[i].Group = "OUTROS"
		}
	}
	return &c, nil
}

func (c *Config) profile(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// Ordem dos grupos = ordem em que aparecem nos perfis.
func (c *Config) groups() []string {
	var gs []string
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		if !seen[p.Group] {
			seen[p.Group] = true
			gs = append(gs, p.Group)
		}
	}
	return append(gs, "OUTROS")
}

// ---- preferências por pasta (perfil lembrado, último uso) ----

type dirPref struct {
	Profile string `json:"profile"`
	Last    int64  `json:"last"`
}

type Prefs struct {
	Dirs map[string]dirPref `json:"dirs"`
}

func loadPrefs() *Prefs {
	p := &Prefs{Dirs: map[string]dirPref{}}
	if data, err := os.ReadFile(prefsPath()); err == nil {
		_ = json.Unmarshal(data, p)
	}
	if p.Dirs == nil {
		p.Dirs = map[string]dirPref{}
	}
	return p
}

func (p *Prefs) save() {
	data, _ := json.MarshalIndent(p, "", "  ")
	tmp := prefsPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, prefsPath())
	}
}

func (c *Config) defaultProfile(dir string, prefs *Prefs) string {
	for _, r := range c.Rules {
		if r.Match != "" && strings.Contains(dir, r.Match) {
			if _, ok := c.profile(r.Profile); ok {
				return r.Profile
			}
		}
	}
	if dp, ok := prefs.Dirs[dir]; ok {
		if _, ok := c.profile(dp.Profile); ok {
			return dp.Profile
		}
	}
	return c.Profiles[0].Name
}
