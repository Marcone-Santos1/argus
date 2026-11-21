package ui

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Marcone-Santos1/argus/internal/i18n"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

func (p *Panel) handleKey(k string) {
	if strings.HasPrefix(k, "mouse:") {
		p.handleMouse(k)
		return
	}
	switch p.mode {
	case modeConfirm:
		fn, no := p.confirmFn, p.confirmNo
		p.mode, p.confirmFn, p.confirmNo = modeList, nil, nil
		if i18n.Yes(k) {
			if fn != nil {
				fn()
			}
		} else if no != nil {
			no()
		}
	case modeAnswer:
		p.keyAnswer(k)
	case modeHelp:
		p.closePicker() // any key closes the help (and leaves the zoom)
	case modeInput:
		p.keyInput(k)
	case modeFilter:
		p.keyFilter(k)
	case modePickDir:
		p.keyPickDir(k)
	case modeHistory:
		p.keyHistory(k)
	case modePickProfile:
		p.keyPickProfile(k)
	default:
		p.keyList(k)
	}
}

// editText applies an editing key to a text; reports whether the key was used.
func editText(s *string, k string) bool {
	switch {
	case k == "backspace":
		if *s != "" {
			_, size := utf8.DecodeLastRuneInString(*s)
			*s = (*s)[:len(*s)-size]
		}
	case k == "ctrl-U":
		*s = ""
	case utf8.RuneCountInString(k) == 1 && k >= " ":
		*s += k
	default:
		return false
	}
	return true
}

func (p *Panel) keyInput(k string) {
	switch k {
	case "esc", "ctrl-C":
		p.mode = modeList
	case "enter":
		fn, v := p.inputFn, p.input
		p.mode, p.inputFn = modeList, nil
		if fn != nil {
			fn(v)
		}
	default:
		editText(&p.input, k)
	}
}

func (p *Panel) keyFilter(k string) {
	switch k {
	case "esc", "ctrl-C":
		p.listFilter = ""
		p.mode = modeList
	case "enter", "down", "up":
		p.mode = modeList
	default:
		if editText(&p.listFilter, k) {
			p.rebuildItems(p.selKey())
		}
		return
	}
	p.rebuildItems(p.selKey())
}

func (p *Panel) keyPickDir(k string) {
	switch k {
	case "esc", "ctrl-C":
		p.closePicker()
	case "up", "ctrl-P":
		if p.pickSel > 0 {
			p.pickSel--
		}
	case "down", "ctrl-N":
		if p.pickSel < len(p.filtered)-1 {
			p.pickSel++
		}
	case "enter":
		if len(p.filtered) == 0 {
			return
		}
		p.chosenDir = p.filtered[p.pickSel].Path
		p.resume = nil
		p.backMode = modePickDir
		p.askProfile(p.d.Config.Profiles, p.d.Config.DefaultProfile(p.chosenDir, p.prefs))
	default:
		if editText(&p.filter, k) {
			p.pickSel = 0
			p.applyFilter()
		}
	}
}

func (p *Panel) keyHistory(k string) {
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
	case "enter":
		if len(p.histShown) > 0 {
			p.resumeEntry(p.histShown[p.histSel])
		}
	default:
		if editText(&p.filter, k) {
			p.histSel = 0
			p.applyHistFilter()
		}
	}
}

func (p *Panel) keyPickProfile(k string) {
	switch k {
	case "esc", "ctrl-C":
		p.switching = ""
		switch p.backMode {
		case modeHistory:
			p.mode = modeHistory
		case modePickDir:
			p.mode = modePickDir
		default:
			p.closePicker()
		}
	case "up", "k":
		if p.profSel > 0 {
			p.profSel--
		}
	case "down", "j":
		if p.profSel < len(p.profChoices)-1 {
			p.profSel++
		}
	case "enter":
		if p.switching != "" {
			p.doSwitch(p.profChoices[p.profSel])
			return
		}
		p.startAgent(p.chosenDir, p.profChoices[p.profSel], p.resume)
	default:
		if len(k) == 1 && k >= "1" && k <= "9" {
			if i := int(k[0] - '1'); i < len(p.profChoices) {
				p.profSel = i
			}
		}
	}
}

func (p *Panel) keyList(k string) {
	switch k {
	case "up", "k":
		if p.sel > 0 {
			p.sel--
			if p.current() != nil {
				p.scheduleSwap()
			}
		}
	case "down", "j":
		if p.sel < len(p.items)-1 {
			p.sel++
			if p.current() != nil {
				p.scheduleSwap()
			}
		}
	case "enter":
		if p.sel >= 0 && p.sel < len(p.items) && p.items[p.sel].agent < 0 {
			if p.items[p.sel].folder {
				p.toggleFolder(nil)
			} else {
				p.resumeEntry(p.items[p.sel].entry)
			}
			return
		}
		p.focusSession()
	case "right", "l":
		yes := true
		p.toggleFolder(&yes)
	case "left":
		no := false
		p.toggleFolder(&no)
	case "y":
		p.startAnswer()
	case "tab":
		p.nextAttention()
	case "n":
		p.openPicker()
	case "w":
		p.startWorktree()
	case "h":
		p.openHistory()
	case "a":
		p.switchAccount()
	case "?":
		p.mode = modeHelp
		p.setZoom(true) // full width, so descriptions are not cut
	case "/":
		p.mode = modeFilter
	case "r":
		p.renameSelected()
	case "esc":
		if p.listFilter != "" {
			p.listFilter = ""
			p.rebuildItems(p.selKey())
			return
		}
		p.hideSession()
	case "x":
		p.killSelected()
	case "z":
		if p.current() != nil {
			p.focusSession()
			_, _ = p.d.Tmux.Run("resize-pane", "-Z", "-t", p.displayPane)
		}
	case ">":
		p.growPanel(stepW)
	case "<":
		p.growPanel(-stepW)
	case "q":
		p.swapC = nil
		_, _ = p.d.Tmux.Run("detach-client", "-s", tmux.Session)
		p.quit = true
	default:
		if len(k) == 1 && k >= "1" && k <= "9" {
			if it := p.nthAgentItem(int(k[0] - '1')); it >= 0 {
				p.sel = it
				p.swapC = nil
				p.showSelected()
			}
		}
	}
}

// ---------- keyboard ----------

func readKeys(ch chan<- string) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			close(ch)
			return
		}
		for _, k := range parseKeys(buf[:n]) {
			ch <- k
		}
	}
}

// parseKeys turns terminal bytes into key names: "up", "enter", "ctrl-U",
// "esc", or the character itself.
func parseKeys(b []byte) []string {
	var ks []string
	for i := 0; i < len(b); {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				if j < len(b) {
					if b[i+2] == '<' && (b[j] == 'M' || b[j] == 'm') {
						if mk := mouseKey(string(b[i+2:j]), b[j]); mk != "" {
							ks = append(ks, mk)
						}
						i = j + 1
						continue
					}
					switch b[j] {
					case 'A':
						ks = append(ks, "up")
					case 'B':
						ks = append(ks, "down")
					case 'C':
						if j == i+2 { // plain arrow, not ctrl/alt+arrow
							ks = append(ks, "right")
						}
					case 'D':
						if j == i+2 { // plain arrow, not ctrl/alt+arrow
							ks = append(ks, "left")
						}
					case 'Z':
						ks = append(ks, "shift-tab")
					}
					i = j + 1
					continue
				}
			}
			ks = append(ks, "esc")
			i++
			continue
		}
		switch {
		case c == '\r' || c == '\n':
			ks = append(ks, "enter")
		case c == 0x7f || c == 0x08:
			ks = append(ks, "backspace")
		case c == '\t':
			ks = append(ks, "tab")
		case c < 0x20:
			ks = append(ks, "ctrl-"+string(rune(c+'@')))
		default:
			r, size := utf8.DecodeRune(b[i:])
			ks = append(ks, string(r))
			i += size
			continue
		}
		i++
	}
	return ks
}
