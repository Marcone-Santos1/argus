package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// item é uma linha selecionável da lista: uma sessão aberta ou uma conversa
// antiga. A lista agrupa tudo por pasta.
type item struct {
	dir   string
	agent int // índice em p.agents; -1 = conversa antiga
	entry histEntry
}

func (it item) key() string {
	if it.agent >= 0 {
		return "a:" // completado por quem tem acesso aos agentes
	}
	return "r:" + it.entry.Tool + ":" + it.entry.ID
}

func (p *panel) itemKey(i int) string {
	if i < 0 || i >= len(p.items) {
		return ""
	}
	it := p.items[i]
	if it.agent >= 0 && it.agent < len(p.agents) {
		return "a:" + p.agents[it.agent].ID
	}
	return it.key()
}

func (p *panel) selKey() string { return p.itemKey(p.sel) }

func (p *panel) selectKey(k string) bool {
	for i := range p.items {
		if p.itemKey(i) == k {
			p.sel = i
			return true
		}
	}
	return false
}

// rebuildItems monta a lista: primeiro as pastas com sessões abertas (sessões e
// depois as conversas antigas daquela pasta), depois as pastas só com conversas
// antigas, na ordem do uso mais recente.
func (p *panel) rebuildItems(keep string) {
	var openDirs []string
	agentsBy := map[string][]int{}
	for i, a := range p.agents {
		if _, ok := agentsBy[a.Dir]; !ok {
			openDirs = append(openDirs, a.Dir)
		}
		agentsBy[a.Dir] = append(agentsBy[a.Dir], i)
	}
	var pastDirs []string
	seen := map[string]bool{}
	recentBy := map[string][]histEntry{}
	for _, e := range p.histAll {
		if _, open := agentsBy[e.Dir]; !open && !seen[e.Dir] {
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
	if p.sel >= len(p.items) {
		p.sel = len(p.items) - 1
	}
	if p.sel < 0 {
		p.sel = 0
	}
}

// nthAgentItem: posição na lista da n-ésima sessão aberta (0 = primeira).
func (p *panel) nthAgentItem(n int) int {
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

func (p *panel) folderHeader(dir string, w int) string {
	name := filepath.Base(dir)
	parent := filepath.Dir(dir)
	if r, err := filepath.Rel(homeDir(), parent); err == nil && !strings.HasPrefix(r, "..") {
		parent = "~/" + r
		if r == "." {
			parent = "~"
		}
	}
	nameW := utf8.RuneCountInString(name)
	if nameW+3 >= w {
		return " " + bold + fit(name, w-1) + reset
	}
	return " " + bold + name + reset + dim + "  " + fit(parent, w-nameW-3) + reset
}

func (p *panel) agentRow(a agent, num int, sel bool, now time.Time, w int) string {
	label := a.Profile
	if pr, ok := p.cfg.profile(a.Profile); ok {
		label = pr.Label()
	}
	text := label + " " + profileTag(p, a.Profile)
	if a.Name != filepath.Base(a.Dir) {
		text += " · " + a.Name
	}
	icon, color := statusIcon(a.Status)
	el := ""
	if !a.Since.IsZero() {
		el = shortDuration(now.Sub(a.Since))
	}
	mark := "  "
	if a.Displayed {
		mark = "▶ "
	}
	n := " "
	if num <= 9 {
		n = itoa(num)
	}
	right := fmt.Sprintf(" %s %4s ", icon, el)
	textW := w - 6 - utf8.RuneCountInString(right)
	if sel {
		return reverse + "  " + n + " " + mark + fit(text, textW) + " " + color + icon + reset + reverse + fmt.Sprintf(" %4s ", el) + reset
	}
	return "  " + dim + n + reset + " " + mark + fit(text, textW) + " " + color + icon + reset + fmt.Sprintf(" %4s ", el)
}

func (p *panel) recentRow(e histEntry, sel bool, now time.Time, w int) string {
	title := e.Title
	right := fmt.Sprintf(" %-3s %4s ", p.accountTag(e), shortDuration(now.Sub(e.Updated)))
	titleW := w - 6 - utf8.RuneCountInString(right)
	if sel {
		return reverse + "      " + fit(title, titleW) + right + reset
	}
	return "      " + fit(title, titleW) + dim + right + reset
}
