package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (p *panel) askProfile(choices []Profile, def string) {
	p.profChoices = choices
	p.profSel = 0
	for i, pr := range choices {
		if pr.Name == def {
			p.profSel = i
		}
	}
	p.mode = modePickProfile
}

func (p *panel) profTitle() string {
	if p.resume != nil {
		return "Retomar conversa: escolha a conta"
	}
	return "Nova sessão: escolha o perfil"
}

// startAgent cria a sessão respeitando o limite de sessões simultâneas.
func (p *panel) startAgent(dir string, prof Profile, resume *histEntry) {
	if len(p.agents) >= p.cfg.MaxSessions {
		p.closePicker()
		p.confirm(fmt.Sprintf("Já são %d sessões (limite %d). Abrir mais uma? (s/n)", len(p.agents), p.cfg.MaxSessions),
			func() { p.createAgent(dir, prof, resume) })
		return
	}
	p.createAgent(dir, prof, resume)
}

func (p *panel) openHistory() {
	p.mode = modeHistory
	p.filter = ""
	p.histSel = 0
	p.resume = nil
	p.setZoom(true)
	t := time.Now()
	p.history = loadHistory(p.cfg)
	p.metrics["history_ms"] = float64(time.Since(t).Microseconds()) / 1000
	p.saveMetrics()
	p.applyHistFilter()
}

func (p *panel) applyHistFilter() {
	terms := strings.Fields(strings.ToLower(p.filter))
	p.histShown = p.histShown[:0]
	for _, e := range p.history {
		hay := strings.ToLower(e.Dir + " " + e.Title + " " + e.Profile + " " + e.Tool)
		ok := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				ok = false
				break
			}
		}
		if ok {
			p.histShown = append(p.histShown, e)
		}
	}
	if p.histSel >= len(p.histShown) {
		p.histSel = len(p.histShown) - 1
	}
	if p.histSel < 0 {
		p.histSel = 0
	}
}

func (p *panel) handleHistoryKey(k string) {
	switch k {
	case "esc", "ctrl-C":
		p.closePicker()
	case "up", "ctrl-P":
		if p.histSel > 0 {
			p.histSel--
		}
	case "down", "ctrl-N":
		if p.histSel < len(p.histShown)-1 {
			p.histSel++
		}
	case "backspace":
		if p.filter != "" {
			_, size := utf8.DecodeLastRuneInString(p.filter)
			p.filter = p.filter[:len(p.filter)-size]
			p.applyHistFilter()
		}
	case "ctrl-U":
		p.filter = ""
		p.applyHistFilter()
	case "enter":
		if len(p.histShown) == 0 {
			return
		}
		p.resumeEntry(p.histShown[p.histSel])
	default:
		if utf8.RuneCountInString(k) == 1 && k >= " " {
			p.filter += k
			p.histSel = 0
			p.applyHistFilter()
		}
	}
}

// Quantas pastas e conversas por pasta aparecem abaixo das sessões.
const (
	recentFolders   = 8
	recentPerFolder = 3
)

// refreshRecent relê o histórico fora do laço principal (o índice deixa isso em ~10 ms).
func (p *panel) refreshRecent() {
	if p.recentLoading {
		return
	}
	p.recentLoading = true
	cfg, ch := p.cfg, p.recentC
	go func() { ch <- loadHistory(cfg) }()
}

func (p *panel) setRecent(all []histEntry) {
	key := p.selKey()
	p.histAll = all
	p.rebuildItems(key)
}

// accountTag: pes / emp / oc (opencode) / ? (conta desconhecida).
func (p *panel) accountTag(e histEntry) string {
	if e.Tool == "opencode" {
		return "oc"
	}
	if e.Profile == "" {
		return "?"
	}
	return profileTag(p, e.Profile)
}

func profileTag(p *panel, profile string) string {
	if pr, ok := p.cfg.profile(profile); ok && pr.Label() == "opencode" {
		return "oc"
	}
	g := []rune(strings.ToLower(p.groupOf(profile)))
	if len(g) > 3 {
		g = g[:3]
	}
	return string(g)
}

// resumeEntry retoma uma conversa; pergunta a conta só quando ela não está registrada.
func (p *panel) resumeEntry(e histEntry) {
	p.backMode = p.mode
	p.resume = &e
	p.chosenDir = e.Dir
	if prof, ok := p.cfg.profile(e.Profile); ok {
		p.startAgent(e.Dir, prof, &e)
		return
	}
	var choices []Profile
	for _, pr := range p.cfg.Profiles {
		if pr.Label() == e.Tool {
			choices = append(choices, pr)
		}
	}
	if len(choices) == 0 {
		p.flash("nenhum perfil configurado para " + e.Tool)
		return
	}
	p.askProfile(choices, choices[0].Name)
}

func (p *panel) renderHistory() []string {
	w := p.w
	body := []string{
		bold + fit(fmt.Sprintf(" Histórico: %d conversas", len(p.history)), w) + reset,
		"",
		fit(" > "+p.filter+"█", w),
		"",
	}
	if len(p.histShown) == 0 {
		body = append(body, dim+fit(" nada encontrado", w)+reset)
	}
	now := time.Now()
	projW := min(22, max(10, w/5))
	profW := 15
	ageW := 4
	titleW := w - 1 - projW - 2 - profW - 2 - ageW - 2
	if titleW < 10 {
		profW = 0
		titleW = w - 1 - projW - 2 - ageW - 2
	}
	for i, e := range p.histShown {
		prof := e.Profile
		if prof == "" {
			prof = e.Tool + " ?"
		}
		proj := filepath.Base(e.Dir)
		age := shortDuration(now.Sub(e.Updated))
		var line string
		if profW > 0 {
			line = " " + fit(proj, projW) + "  " + fit(e.Title, titleW) + "  " + fit(prof, profW) + "  " + fmt.Sprintf("%*s", ageW, age)
		} else {
			line = " " + fit(proj, projW) + "  " + fit(e.Title, titleW) + "  " + fmt.Sprintf("%*s", ageW, age)
		}
		if i == p.histSel {
			line = reverse + fit(line, w) + reset
		} else if e.Profile == "" {
			line = dim + fit(line, w) + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"digite para filtrar (projeto, título, conta)", "↑↓ escolher", "⏎ retomar", "esc volta"}, w)...)
	return layout(body, foot, p.h, p.histSel+4)
}
