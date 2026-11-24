package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
)

// How many folders with past conversations the list shows, and how many
// conversations an opened folder shows.
const (
	recentFolders   = 10
	recentPerFolder = 5
)

// item is a selectable line: an open session, a folder of past
// conversations, or one of its conversations (when the folder is open).
type item struct {
	dir    string
	agent  int  // index in p.agents; -1 = not a session
	folder bool // a folder row (entry is its newest conversation)
	count  int  // folder: how many conversations it has
	entry  history.Entry
}

func (p *Panel) itemKey(i int) string {
	if i < 0 || i >= len(p.items) {
		return ""
	}
	it := p.items[i]
	switch {
	case it.agent >= 0 && it.agent < len(p.agents):
		return "a:" + p.agents[it.agent].ID
	case it.folder:
		return "f:" + it.dir
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

// rebuildItems builds the list in two parts: the open sessions, oldest first;
// then one row per folder with past conversations, most recently used first,
// with its conversations under it when the folder is open (or a filter is
// typed). Conversations already open in a session are not repeated.
func (p *Panel) rebuildItems(keep string) {
	open := map[string]bool{}
	old := p.sel
	p.items = p.items[:0]
	// Sessions are grouped by folder; folders keep the order of their oldest
	// session, sessions keep theirs inside the folder.
	var groupOrder []string
	groups := map[string][]int{}
	for i, a := range p.agents {
		if a.Session != "" {
			open[a.Session] = true
		}
		if p.listFilter != "" && !matchTerms(a.Dir+" "+a.Name+" "+a.Profile+" "+p.titleOf[a.Session], p.listFilter) {
			continue
		}
		if _, ok := groups[a.Dir]; !ok {
			groupOrder = append(groupOrder, a.Dir)
		}
		groups[a.Dir] = append(groups[a.Dir], i)
	}
	for _, d := range groupOrder {
		for _, i := range groups[d] {
			p.items = append(p.items, item{dir: d, agent: i})
		}
	}
	sessions := len(p.items)
	var dirs []string
	byDir := map[string][]history.Entry{}
	count := map[string]int{}
	hidden := map[string]bool{}
	for _, e := range p.histAll {
		if isTemp(e.Dir) {
			continue
		}
		if p.listFilter != "" && !matchTerms(e.Dir+" "+e.Title+" "+e.Profile+" "+e.Tool, p.listFilter) {
			continue
		}
		// A folder's place is its newest conversation, the open ones
		// included (it is in use), but those are not listed again.
		if _, seen := byDir[e.Dir]; !seen {
			if len(dirs) == recentFolders {
				hidden[e.Dir] = true
				continue
			}
			dirs = append(dirs, e.Dir)
			byDir[e.Dir] = nil
		}
		if open[e.ID] {
			continue
		}
		count[e.Dir]++
		if len(byDir[e.Dir]) < recentPerFolder {
			byDir[e.Dir] = append(byDir[e.Dir], e)
		}
	}
	p.hiddenFolders = len(hidden)
	for _, d := range dirs {
		es := byDir[d]
		if len(es) == 0 {
			continue
		}
		p.items = append(p.items, item{dir: d, agent: -1, folder: true, count: count[d], entry: es[0]})
		if p.expanded[d] || p.listFilter != "" {
			for _, e := range es {
				p.items = append(p.items, item{dir: d, agent: -1, entry: e})
			}
		}
	}
	if p.appLook() {
		p.items = p.appItems(append([]item{}, p.items[:sessions]...), append([]item{}, p.items[sessions:]...))
	}
	if keep == "" || !p.selectKey(keep) {
		// A conversation of a folder that closed: select the folder.
		if !strings.HasPrefix(keep, "r:") || !p.selectFolderOf(keep) {
			p.sel = old
		}
	}
	p.sel = clamp(p.sel, len(p.items))
}

// selectFolderOf selects the folder row of a conversation key ("r:...").
func (p *Panel) selectFolderOf(key string) bool {
	for _, e := range p.histAll {
		if "r:"+e.Key() == key {
			return p.selectKey("f:" + e.Dir)
		}
	}
	return false
}

// toggleFolder opens or closes the selected folder (or the folder of the
// selected conversation); open=nil flips it.
func (p *Panel) toggleFolder(open *bool) {
	if p.sel < 0 || p.sel >= len(p.items) || p.items[p.sel].agent >= 0 {
		return
	}
	it := p.items[p.sel]
	want := !p.expanded[it.dir]
	if open != nil {
		want = *open
	}
	if want {
		p.expanded[it.dir] = true
	} else {
		delete(p.expanded, it.dir)
	}
	p.rebuildItems("f:" + it.dir)
}

// nthAgentItem is the list position of the n-th open session (0 = first).
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

// ---------- projects (key n) ----------

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
