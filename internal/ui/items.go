package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
)

// Quantas pastas só com conversas antigas, e quantas conversas por pasta, a lista mostra.
const (
	recentFolders   = 8
	recentPerFolder = 3
)

// item é uma linha selecionável: uma sessão aberta ou uma conversa antiga.
type item struct {
	dir   string
	agent int // índice em p.agents; -1 = conversa antiga
	entry history.Entry
}

func (p *Panel) itemKey(i int) string {
	if i < 0 || i >= len(p.items) {
		return ""
	}
	it := p.items[i]
	if it.agent >= 0 && it.agent < len(p.agents) {
		return "a:" + p.agents[it.agent].ID
	}
	return "r:" + it.entry.Key()
}

func (p *Panel) selKey() string { return p.itemKey(p.sel) }

func (p *Panel) selectKey(k string) bool {
	for i := range p.items {
		if p.itemKey(i) == k {
			p.sel = i
			return true
		}
	}
	return false
}

// rebuildItems monta a lista agrupada por pasta: primeiro as pastas com sessões
// abertas (as sessões e, embaixo, as conversas antigas daquela pasta), depois as
// pastas só com conversas antigas, na ordem do uso mais recente. Conversas que
// já estão abertas numa sessão não se repetem. O filtro (/) vale para tudo.
func (p *Panel) rebuildItems(keep string) {
	open := map[string]bool{}
	var openDirs []string
	agentsBy := map[string][]int{}
	for i, a := range p.agents {
		if a.Session != "" {
			open[a.Session] = true
		}
		if p.listFilter != "" && !matchTerms(a.Dir+" "+a.Name+" "+a.Profile, p.listFilter) {
			continue
		}
		if _, ok := agentsBy[a.Dir]; !ok {
			openDirs = append(openDirs, a.Dir)
		}
		agentsBy[a.Dir] = append(agentsBy[a.Dir], i)
	}
	var pastDirs []string
	seen := map[string]bool{}
	recentBy := map[string][]history.Entry{}
	for _, e := range p.histAll {
		if open[e.ID] {
			continue
		}
		if p.listFilter != "" && !matchTerms(e.Dir+" "+e.Title+" "+e.Profile+" "+e.Tool, p.listFilter) {
			continue
		}
		if _, isOpen := agentsBy[e.Dir]; !isOpen && !seen[e.Dir] {
			if len(pastDirs) == recentFolders {
				continue
			}
			seen[e.Dir] = true
			pastDirs = append(pastDirs, e.Dir)
		}
		if len(recentBy[e.Dir]) < recentPerFolder {
			recentBy[e.Dir] = append(recentBy[e.Dir], e)
		}
	}
	old := p.sel
	p.items = p.items[:0]
	for _, d := range openDirs {
		for _, i := range agentsBy[d] {
			p.items = append(p.items, item{dir: d, agent: i})
		}
		for _, e := range recentBy[d] {
			p.items = append(p.items, item{dir: d, agent: -1, entry: e})
		}
	}
	for _, d := range pastDirs {
		for _, e := range recentBy[d] {
			p.items = append(p.items, item{dir: d, agent: -1, entry: e})
		}
	}
	if keep == "" || !p.selectKey(keep) {
		p.sel = old
	}
	p.sel = clamp(p.sel, len(p.items))
}

// nthAgentItem: posição na lista da n-ésima sessão aberta (0 = primeira).
func (p *Panel) nthAgentItem(n int) int {
	for i, it := range p.items {
		if it.agent >= 0 {
			if n == 0 {
				return i
			}
			n--
		}
	}
	return -1
}

// ---------- projetos (tecla n) ----------

type project struct {
	Path, Rel string
	Last      int64
}

func scanProjects(cfg *config.Config, prefs *config.Prefs, home string) []project {
	seen := map[string]bool{}
	var out []project
	add := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		rel, err := filepath.Rel(home, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = path
		}
		out = append(out, project{Path: path, Rel: rel, Last: prefs.Dirs[path].Last})
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			n := e.Name()
			full := filepath.Join(dir, n)
			if strings.HasPrefix(n, ".") || n == "node_modules" || !isDir(full, e) {
				continue
			}
			add(full)
			if depth > 1 && !exists(filepath.Join(full, ".git")) {
				walk(full, depth-1)
			}
		}
	}
	for _, r := range cfg.Roots {
		walk(r, cfg.Depth)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Last != out[j].Last {
			return out[i].Last > out[j].Last
		}
		return strings.ToLower(out[i].Rel) < strings.ToLower(out[j].Rel)
	})
	return out
}

func isDir(path string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink != 0 {
		fi, err := os.Stat(path)
		return err == nil && fi.IsDir()
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
