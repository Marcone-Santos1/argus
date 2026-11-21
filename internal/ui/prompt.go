package ui

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/status"
)

// prompt is what a session waiting for you is asking, read from its screen:
// a permission request or a question with a list of options.
type prompt struct {
	Context  string   // what it is about (the command, the file, the title)
	Question string   // "Do you want to proceed?"
	Options  []string // option texts, in order
	Numbered bool     // options are typed as 1, 2, 3...
	Selected int      // option under the cursor (❯)
}

var (
	numberedRe = regexp.MustCompile(`^\s*(?:[❯›>]\s*)?(\d+)\.\s+(.*\S)\s*$`)
	cursorRe   = regexp.MustCompile(`^\s*[❯›>]\s+(.*\S)\s*$`)
)

func isRule(l string) bool {
	t := strings.TrimSpace(l)
	return t != "" && strings.Trim(t, "─━╌┄-") == ""
}

func isHint(l string) bool {
	for _, h := range []string{"Esc to cancel", "Enter to confirm", "Tab to amend", "to cancel ·", "↑/↓"} {
		if strings.Contains(l, h) {
			return true
		}
	}
	return false
}

func indent(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

// parsePrompt finds the last option list on a captured screen. Claude Code
// numbers its options ("❯ 1. Yes"); other prompts only mark the current one
// with ❯. Lines more indented than an option continue it.
func parsePrompt(screen string) (prompt, bool) {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	end := len(lines) - 1
	for end >= 0 && (strings.TrimSpace(lines[end]) == "" || isHint(lines[end])) {
		end--
	}
	if end < 0 {
		return prompt{}, false
	}

	var pr prompt
	start := -1
	// Numbered options: walk up over the block (options and their continuations).
	for i := end; i >= 0 && i >= end-30; i-- {
		if numberedRe.MatchString(lines[i]) {
			start = i
			continue
		}
		if start != -1 && strings.TrimSpace(lines[i]) != "" && indent(lines[i]) <= indent(lines[start]) {
			break
		}
	}
	if start != -1 {
		// The list must be what the screen ends with, not a numbered list
		// somewhere in the conversation above.
		lastOpt := start
		for i := start; i <= end; i++ {
			if numberedRe.MatchString(lines[i]) {
				lastOpt = i
			}
		}
		if end-lastOpt > 3 {
			return prompt{}, false
		}
		pr.Numbered = true
		optIndent := -1
		for i := start; i <= end; i++ {
			l := lines[i]
			if m := numberedRe.FindStringSubmatch(l); m != nil && (optIndent == -1 || indent(l) <= optIndent+2) {
				optIndent = indent(l)
				if n, _ := strconv.Atoi(m[1]); n != len(pr.Options)+1 {
					return prompt{}, false // not a 1..n list
				}
				if cursorRe.MatchString(l) {
					pr.Selected = len(pr.Options)
				}
				pr.Options = append(pr.Options, m[2])
			} else if t := strings.TrimSpace(l); t != "" && len(pr.Options) > 0 {
				pr.Options[len(pr.Options)-1] += " " + t
			}
		}
	} else {
		// Unnumbered: the cursor line and its siblings at the same indentation.
		cur := -1
		for i := end; i >= 0 && i >= end-30; i-- {
			if cursorRe.MatchString(lines[i]) {
				cur = i
				break
			}
		}
		if cur == -1 {
			return prompt{}, false
		}
		col := strings.IndexAny(lines[cur], "❯›>")
		start = cur
		for start-1 >= 0 && sibling(lines[start-1], col) {
			start--
		}
		last := cur
		for last+1 <= end && sibling(lines[last+1], col) {
			last++
		}
		for i := start; i <= last; i++ {
			if i == cur {
				pr.Selected = i - start
			}
			pr.Options = append(pr.Options, strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i]), "❯›>")))
		}
		if len(pr.Options) < 2 {
			return prompt{}, false
		}
	}

	// The question: the nearest text above the options.
	q := start - 1
	for q >= 0 && strings.TrimSpace(lines[q]) == "" {
		q--
	}
	if q >= 0 && !isRule(lines[q]) {
		pr.Question = strings.TrimSpace(lines[q])
	}
	// The context: a line framed by ╌ rules (the command), or else the first
	// line after the last full rule (the dialog's title).
	top := -1
	for i := q - 1; i >= 0 && i >= q-25; i-- {
		if isRule(lines[i]) && !strings.ContainsRune(lines[i], '╌') {
			top = i
			break
		}
	}
	for i := top + 1; i < q && i >= 0; i++ {
		if strings.ContainsRune(lines[i], '╌') && i+1 < q && strings.TrimSpace(lines[i+1]) != "" && !isRule(lines[i+1]) {
			pr.Context = strings.TrimSpace(lines[i+1])
			break
		}
	}
	if pr.Context == "" && top != -1 {
		for i := top + 1; i < q; i++ {
			if t := strings.TrimSpace(lines[i]); t != "" && !isRule(lines[i]) {
				pr.Context = t
				break
			}
		}
	}
	return pr, true
}

func sibling(l string, col int) bool {
	if strings.TrimSpace(l) == "" || isHint(l) || isRule(l) {
		return false
	}
	// Options without the cursor are indented past it ("❯ " is two columns).
	return indent(l) == col+2
}

// short cuts an option to a few words for the list.
func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// capturePrompt reads a waiting session's screen and keeps what it asks.
func (p *Panel) capturePrompt(agentID string) {
	for _, a := range p.agents {
		if a.ID != agentID {
			continue
		}
		if a.Status != status.Waiting {
			delete(p.prompts, agentID)
			return
		}
		out, err := p.d.Tmux.Run("capture-pane", "-p", "-J", "-t", a.Pane, "-S", "-40")
		if err != nil {
			return
		}
		if pr, ok := parsePrompt(out); ok {
			p.prompts[agentID] = pr
		} else {
			delete(p.prompts, agentID)
		}
	}
}

// capturePromptSoon reads the screen a moment after the session starts
// waiting: the hook fires before the agent finishes drawing the question.
func (p *Panel) capturePromptSoon(agentID string) {
	ch := p.captureC
	time.AfterFunc(400*time.Millisecond, func() {
		select {
		case ch <- agentID:
		default:
		}
	})
}

// startAnswer opens the answer mode for the selected waiting session (key y).
func (p *Panel) startAnswer() {
	a := p.current()
	if a == nil || a.Status != status.Waiting {
		p.flash(i18n.T("msg.notWaiting"))
		return
	}
	p.capturePrompt(a.ID) // what is on screen now, not what was there before
	pr, ok := p.prompts[a.ID]
	if !ok || len(pr.Options) == 0 {
		p.flash(i18n.T("msg.noPrompt"))
		return
	}
	p.answering = a.ID
	p.mode = modeAnswer
}

// answer sends the choice to the session: the option's number when the
// prompt is numbered, otherwise arrows to it and Enter.
func (p *Panel) answer(choice int) {
	id := p.answering
	p.answering = ""
	p.mode = modeList
	var a *agent
	for i := range p.agents {
		if p.agents[i].ID == id {
			a = &p.agents[i]
		}
	}
	pr, ok := p.prompts[id]
	if a == nil || !ok || choice < 0 || choice >= len(pr.Options) {
		return
	}
	var err error
	if pr.Numbered {
		_, err = p.d.Tmux.Run("send-keys", "-t", a.Pane, "-l", strconv.Itoa(choice+1))
	} else {
		args := []string{"send-keys", "-t", a.Pane}
		key := "Down"
		moves := choice - pr.Selected
		if moves < 0 {
			key, moves = "Up", -moves
		}
		for i := 0; i < moves; i++ {
			args = append(args, key)
		}
		_, err = p.d.Tmux.Run(append(args, "Enter")...)
	}
	if err != nil {
		p.flash(err.Error())
		return
	}
	delete(p.prompts, id)
	p.flash(i18n.T("msg.answered", a.Name, short(pr.Options[choice], 40)))
}

func (p *Panel) keyAnswer(k string) {
	pr := p.prompts[p.answering]
	switch {
	case k == "esc" || k == "ctrl-C" || k == "q":
		p.answering = ""
		p.mode = modeList
	case k == "enter":
		p.answer(pr.Selected)
	case len(k) == 1 && k >= "1" && k <= "9":
		if i := int(k[0] - '1'); i < len(pr.Options) {
			p.answer(i)
		}
	}
}
