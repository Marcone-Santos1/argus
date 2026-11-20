// Package config lê a configuração do Argus (perfis, pastas de projetos) e as
// preferências lembradas por pasta.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Profile é um comando com seu ambiente (uma conta do Claude, o opencode...).
type Profile struct {
	Name    string            `toml:"name"`
	Group   string            `toml:"group"`
	Command string            `toml:"command"`
	Env     map[string]string `toml:"env"`
}

// Label é o nome curto do programa (claude, opencode...).
func (p Profile) Label() string {
	f := strings.Fields(p.Command)
	if len(f) == 0 {
		return p.Name
	}
	return filepath.Base(f[0])
}

// ClaudeConfigDir é a pasta de config do Claude usada pelo perfil.
func (p Profile) ClaudeConfigDir(home string) string {
	if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
		return v
	}
	return filepath.Join(home, ".claude")
}

// ClaudeStateFile é o .claude.json do perfil (onde fica a conta logada).
func (p Profile) ClaudeStateFile(home string) string {
	if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
		return filepath.Join(v, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

// Tag é a etiqueta curta de conta mostrada na lista: pes, emp, oc...
func (p Profile) Tag() string {
	if p.Label() == "opencode" {
		return "oc"
	}
	g := []rune(strings.ToLower(p.Group))
	if len(g) > 3 {
		g = g[:3]
	}
	return string(g)
}

// Rule escolhe um perfil para pastas que contêm Match.
type Rule struct {
	Match   string `toml:"match"`
	Profile string `toml:"profile"`
}

type Config struct {
	Roots       []string  `toml:"roots"`
	Depth       int       `toml:"depth"`
	MaxSessions int       `toml:"max_sessions"`
	Notify      []string  `toml:"notify"`
	Profiles    []Profile `toml:"profile"`
	Rules       []Rule    `toml:"rule"`
}

// DefaultTOML é o config.toml criado pelo install.
const DefaultTOML = `# Argus: configuração. Este arquivo é seu; o install não o sobrescreve.

# Pastas onde o "n" procura projetos, e quantos níveis descer.
roots = ["~/Projects", "~/Sites"]
depth = 2

# Acima disso, criar sessão pede confirmação.
max_sessions = 8

# Avisos do macOS quando uma sessão muda para estes estados
# (waiting = esperando você, done = terminou). Lista vazia desliga.
notify = ["waiting", "done"]

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

// Load lê o config.toml (ou o padrão, se ele não existir) e expande "~".
func Load(path, home string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte(DefaultTOML)
	}
	return Parse(string(data), home)
}

// Parse interpreta um config.toml.
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
	if !md.IsDefined("notify") {
		c.Notify = []string{"waiting", "done"}
	}
	if len(c.Profiles) == 0 {
		c.Profiles = []Profile{{Name: "claude", Group: "PESSOAL", Command: "claude"}}
	}
	for i := range c.Profiles {
		if c.Profiles[i].Group == "" {
			c.Profiles[i].Group = "OUTROS"
		}
		for k, v := range c.Profiles[i].Env {
			c.Profiles[i].Env[k] = expand(v)
		}
	}
	return &c, nil
}

// Profile procura um perfil pelo nome.
func (c *Config) Profile(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// ProfilesFor lista os perfis de um programa (claude, opencode).
func (c *Config) ProfilesFor(label string) []Profile {
	var ps []Profile
	for _, p := range c.Profiles {
		if p.Label() == label {
			ps = append(ps, p)
		}
	}
	return ps
}

// ShouldNotify diz se uma mudança para o estado s gera aviso.
func (c *Config) ShouldNotify(s string) bool {
	for _, n := range c.Notify {
		if n == s {
			return true
		}
	}
	return false
}

// DefaultProfile escolhe o perfil de uma pasta: regra, depois o último usado
// nela, depois o primeiro perfil.
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

// DirPref é o que o Argus lembra de uma pasta.
type DirPref struct {
	Profile string `json:"profile"`
	Last    int64  `json:"last"`
}

// Prefs são as preferências lembradas por pasta.
type Prefs struct {
	Dirs map[string]DirPref `json:"dirs"`
	path string
}

// LoadPrefs lê as preferências; um arquivo ausente ou inválido vira vazio.
func LoadPrefs(path string) *Prefs {
	p := &Prefs{Dirs: map[string]DirPref{}, path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, p)
	}
	if p.Dirs == nil {
		p.Dirs = map[string]DirPref{}
	}
	return p
}

// Save grava as preferências de forma atômica.
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
