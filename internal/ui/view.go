package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/status"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	reverse = "\x1b[7m"
	red     = "\x1b[1;31m"
	yellow  = "\x1b[1;33m"
	cyan    = "\x1b[36m"
)

func statusIcon(s string) (string, string) {
	switch s {
	case status.Working:
		return "●", "\x1b[33m"
	case status.Waiting:
		return "◐", "\x1b[1;35m"
	case status.Done:
		return "✓", "\x1b[32m"
	case status.Ended:
		return "✕", "\x1b[31m"
	default:
		return "○", "\x1b[2m"
	}
}

func (p *Panel) resize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 10 || h < 4 {
		w, h = 40, 20
	}
	p.w, p.h = w, h
}

// fit corta ou completa com espaços até w colunas.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n <= w {
		return s + strings.Repeat(" ", w-n)
	}
	r := []rune(s)
	return string(r[:w-1]) + "…"
}

func (p *Panel) render() {
	var lines []string
	switch p.mode {
	case modePickDir:
		lines = p.viewPicker()
	case modePickProfile:
		lines = p.viewProfiles()
	case modeHistory:
		lines = p.viewHistory()
	default:
		lines = p.viewList()
	}
	b := p.out
	b.WriteString("\x1b[H")
	for i := 0; i < p.h; i++ {
		if i > 0 {
			b.WriteString("\r\n")
		}
		// Limpa a linha antes de escrever: limpar depois apagaria a última
		// coluna (sem quebra automática o cursor fica sobre ela).
		b.WriteString("\x1b[2K")
		if i < len(lines) {
			b.WriteString(lines[i])
		}
		b.WriteString(reset)
	}
	_ = b.Flush()

	if !p.rendered {
		p.rendered = true
		p.metrics["startup_ms"] = float64(time.Since(p.d.Started).Microseconds()) / 1000
		p.saveMetrics()
	}
	if !p.swapStart.IsZero() {
		p.metrics["switch_ms"] = float64(time.Since(p.swapStart).Microseconds()) / 1000
		p.swapStart = time.Time{}
		p.saveMetrics()
	}
	if p.benchAck != "" {
		_ = os.WriteFile(filepath.Join(p.d.Paths.State, "_bench.ack"), []byte(p.benchAck), 0o644)
		p.benchAck = ""
	}
}

func (p *Panel) viewList() []string {
	w := p.w
	body := []string{bold + fit(" argus", w) + reset}
	if p.mode == modeFilter || p.listFilter != "" {
		cursor := ""
		if p.mode == modeFilter {
			cursor = "█"
		}
		body = append(body, fit(" / "+p.listFilter+cursor, w))
	}
	if len(p.agents) == 0 && p.listFilter == "" {
		body = append(body, dim+fit(" Nenhuma sessão aberta. n cria uma;", w)+reset, dim+fit(" Enter numa conversa a retoma.", w)+reset)
	}
	focusLine := -1
	now := time.Now()
	dir := ""
	num := 0
	for idx, it := range p.items {
		if it.dir != dir {
			dir = it.dir
			body = append(body, "", p.folderHeader(dir, w))
		}
		sel := idx == p.sel && p.mode != modeFilter
		if idx == p.sel {
			focusLine = len(body)
		}
		if it.agent >= 0 {
			num++
			body = append(body, p.agentRow(p.agents[it.agent], num, sel, now, w))
		} else {
			body = append(body, p.recentRow(it.entry, sel, now, w))
		}
	}
	if len(p.items) == 0 && p.listFilter != "" {
		body = append(body, "", dim+fit(" nada encontrado", w)+reset)
	}
	return layout(body, p.footer(), p.h, max(focusLine, 0))
}

func (p *Panel) footer() []string {
	w := p.w
	var foot []string
	switch {
	case p.mode == modeConfirm:
		foot = append(foot, yellow+fit(" "+p.confirmMsg, w)+reset)
	case p.mode == modeInput:
		foot = append(foot, yellow+fit(" "+p.inputPrompt, w)+reset, fit(" > "+p.input+"█", w))
	case p.msg != "":
		foot = append(foot, cyan+fit(" "+p.msg, w)+reset)
	}
	if p.reading.Storm {
		foot = append(foot, red+fit(fmt.Sprintf(" ⚠ %.0f processos/min", p.reading.PerMinute), w)+reset)
	}
	foot = append(foot, dim+strings.Repeat("─", w)+reset)
	sess := fmt.Sprintf("%d sessões", len(p.agents))
	if len(p.agents) == 1 {
		sess = "1 sessão"
	}
	stats := ""
	if p.reading.CPU >= 0 && p.reading.PerMinute >= 0 {
		stats = fmt.Sprintf("CPU %.0f%%  proc/min %.0f  ", p.reading.CPU, p.reading.PerMinute)
	}
	foot = append(foot, fit(" "+stats+sess, w))
	hints := []string{"⏎ entrar", "^\\ alterna", "tab atenção", "n nova", "h histórico", "w worktree", "/ filtra", "r nome", "esc esconde", "x fecha", "z cheia", "q sai"}
	return append(foot, wrapHints(hints, w)...)
}

func (p *Panel) folderHeader(dir string, w int) string {
	name := filepath.Base(dir)
	parent := p.d.Paths.Pretty(filepath.Dir(dir))
	nameW := utf8.RuneCountInString(name)
	if nameW+3 >= w {
		return " " + bold + fit(name, w-1) + reset
	}
	return " " + bold + name + reset + dim + "  " + fit(parent, w-nameW-3) + reset
}

func (p *Panel) agentRow(a agent, num int, sel bool, now time.Time, w int) string {
	label, tag := a.Profile, "?"
	if pr, ok := p.profileOf(a.Profile); ok {
		label, tag = pr.Label(), pr.Tag()
	}
	text := label + " " + tag
	if a.Worktree != "" {
		text += " ⑂"
	}
	if a.Name != filepath.Base(a.Dir) {
		text += " · " + a.Name
	}
	icon, color := statusIcon(a.Status)
	el := ""
	if !a.Since.IsZero() {
		el = status.Short(now.Sub(a.Since))
	}
	mark := "  "
	if a.Displayed {
		mark = "▶ "
	}
	n := " "
	if num <= 9 {
		n = fmt.Sprint(num)
	}
	right := fmt.Sprintf(" %s %4s ", icon, el)
	textW := w - 6 - utf8.RuneCountInString(right)
	if sel {
		return reverse + "  " + n + " " + mark + fit(text, textW) + " " + color + icon + reset + reverse + fmt.Sprintf(" %4s ", el) + reset
	}
	return "  " + dim + n + reset + " " + mark + fit(text, textW) + " " + color + icon + reset + fmt.Sprintf(" %4s ", el)
}

func (p *Panel) entryTag(e history.Entry) string {
	if e.Tool == history.Opencode {
		return "oc"
	}
	if pr, ok := p.profileOf(e.Profile); ok {
		return pr.Tag()
	}
	return "?"
}

func (p *Panel) recentRow(e history.Entry, sel bool, now time.Time, w int) string {
	right := fmt.Sprintf(" %-3s %4s ", p.entryTag(e), status.Short(now.Sub(e.Updated)))
	titleW := w - 6 - utf8.RuneCountInString(right)
	if sel {
		return reverse + "      " + fit(e.Title, titleW) + right + reset
	}
	return "      " + fit(e.Title, titleW) + dim + right + reset
}

func wrapHints(hints []string, w int) []string {
	var lines []string
	cur := ""
	for _, h := range hints {
		cand := cur + "  " + h
		if cur == "" {
			cand = " " + h
		}
		if utf8.RuneCountInString(cand) > w && cur != "" {
			lines = append(lines, dim+fit(cur, w)+reset)
			cand = " " + h
		}
		cur = cand
	}
	if cur != "" {
		lines = append(lines, dim+fit(cur, w)+reset)
	}
	return lines
}

// layout junta corpo e rodapé na altura da tela, rolando o corpo para manter
// a linha focus visível.
func layout(body, foot []string, h, focus int) []string {
	room := max(h-len(foot), 1)
	if len(body) > room {
		start := min(max(focus-room/2, 0), len(body)-room)
		body = body[start : start+room]
	}
	out := append([]string{}, body...)
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, foot...)
}

func (p *Panel) viewPicker() []string {
	w := p.w
	body := []string{
		bold + fit(" Nova sessão: escolha o projeto", w) + reset, "",
		fit(" > "+p.filter+"█", w), "",
	}
	if len(p.filtered) == 0 {
		body = append(body, dim+fit(" nada encontrado", w)+reset)
	}
	for i, pr := range p.filtered {
		mark := "  "
		if pr.Last > 0 {
			mark = "· "
		}
		line := fit(" "+mark+pr.Rel, w)
		if i == p.pickSel {
			line = reverse + line + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"digite para filtrar", "↑↓ escolher", "⏎ ok", "esc cancela", "· usado antes"}, w)...)
	return layout(body, foot, p.h, p.pickSel+4)
}

func (p *Panel) viewProfiles() []string {
	w := p.w
	title := "Nova sessão: escolha o perfil"
	switch {
	case p.resume != nil:
		title = "Retomar conversa: escolha a conta"
	case p.worktreeOf != "":
		title = "Sessão na worktree: escolha o perfil"
	}
	body := []string{bold + fit(" "+title, w) + reset, dim + fit(" "+p.d.Paths.Pretty(p.chosenDir), w) + reset, ""}
	for i, pr := range p.profChoices {
		text := fmt.Sprintf(" %d  %-16s %s", i+1, pr.Name, pr.Group)
		if i == p.profSel {
			body = append(body, reverse+fit(text, w)+reset)
		} else {
			body = append(body, fit(text, w))
		}
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"↑↓ escolher", "⏎ abrir", "esc voltar"}, w)...)
	return layout(body, foot, p.h, p.profSel+3)
}

func (p *Panel) viewHistory() []string {
	w := p.w
	body := []string{
		bold + fit(fmt.Sprintf(" Histórico: %d conversas", len(p.histAll)), w) + reset, "",
		fit(" > "+p.filter+"█", w), "",
	}
	if len(p.histShown) == 0 {
		body = append(body, dim+fit(" nada encontrado", w)+reset)
	}
	now := time.Now()
	projW := min(22, max(10, w/5))
	const ageW = 4
	tagW := 4
	titleW := w - 1 - projW - 2 - tagW - 1 - ageW - 1
	for i, e := range p.histShown {
		line := " " + fit(filepath.Base(e.Dir), projW) + "  " + fit(e.Title, titleW) + " " +
			fit(p.entryTag(e), tagW) + fmt.Sprintf("%*s ", ageW, status.Short(now.Sub(e.Updated)))
		switch {
		case i == p.histSel:
			line = reverse + fit(line, w) + reset
		case e.Profile == "":
			line = dim + fit(line, w) + reset
		}
		body = append(body, line)
	}
	foot := []string{dim + strings.Repeat("─", w) + reset}
	foot = append(foot, wrapHints([]string{"digite para filtrar (projeto, título, conta)", "↑↓ escolher", "⏎ retomar", "esc volta"}, w)...)
	return layout(body, foot, p.h, p.histSel+4)
}
