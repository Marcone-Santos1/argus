package ui

import (
	"sort"
	"strconv"
)

// ---------- moving sessions in the list (Shift+↑↓ or K/J) ----------

// rank is a session's place in the list: the place it was moved to, or its
// creation time. Moved places are small numbers, so a new session lands at
// the end.
func (a agent) rank() int64 {
	if a.Order > 0 {
		return a.Order
	}
	return a.Created
}

func sortAgents(as []agent) {
	sort.SliceStable(as, func(i, j int) bool { return as[i].rank() < as[j].rank() })
}

// groupedOrder is the sessions as the list shows them: grouped by folder,
// folders in the order of their first session.
func groupedOrder(as []agent) [][]int {
	var groups [][]int
	at := map[string]int{}
	for i, a := range as {
		g, ok := at[a.Dir]
		if !ok {
			g = len(groups)
			at[a.Dir] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	return groups
}

// moveSession moves the selected session one place up (-1) or down (+1)
// inside its folder; at the folder's edge the whole folder moves past the
// next one. The new order is kept in each pane (it survives the panel).
func (p *Panel) moveSession(dir int) {
	a := p.current()
	if a == nil {
		return
	}
	groups := groupedOrder(p.agents)
	g, pos := -1, -1
	for gi, grp := range groups {
		for pi, i := range grp {
			if p.agents[i].ID == a.ID {
				g, pos = gi, pi
			}
		}
	}
	if g < 0 {
		return
	}
	switch np := pos + dir; {
	case np >= 0 && np < len(groups[g]):
		groups[g][pos], groups[g][np] = groups[g][np], groups[g][pos]
	case g+dir >= 0 && g+dir < len(groups):
		groups[g], groups[g+dir] = groups[g+dir], groups[g]
	default:
		return
	}
	n := int64(0)
	for _, grp := range groups {
		for _, i := range grp {
			n++
			if p.agents[i].Order != n {
				p.agents[i].Order = n
				_ = p.d.Tmux.SetPaneOptions(p.agents[i].Pane, "@argus_order", strconv.FormatInt(n, 10))
			}
		}
	}
	key := p.selKey()
	sortAgents(p.agents)
	p.rebuildItems(key)
	p.saveSnapshot()
}
