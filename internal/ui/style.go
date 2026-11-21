package ui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Marcone-Santos1/argus/internal/status"
)

// Palette in 24-bit color. tmux converts to 256 colors when the outer terminal
// has no truecolor, so it degrades gracefully.
const (
	cAccent  = "\x1b[38;2;122;162;247m" // blue: brand, selection bar
	cFolder  = "\x1b[1;38;2;97;175;239m"
	cQuiet   = "\x1b[38;2;108;113;124m" // secondary text
	cFaint   = "\x1b[38;2;74;79;89m"    // tree guides, paths
	cWorking = "\x1b[38;2;229;192;123m"
	cWaiting = "\x1b[1;38;2;209;154;236m"
	cDone    = "\x1b[38;2;152;195;121m"
	cEnded   = "\x1b[38;2;224;108;117m"
	cAmber   = "\x1b[38;2;229;192;123m"
	bgSel    = "\x1b[48;2;43;49;60m"
	bgWait   = "\x1b[48;2;58;48;32m"
)

// span is a piece of a line with its own style.
type span struct{ text, style string }

func sp(text, style string) span { return span{text, style} }

// width counts columns (all our symbols are one column wide).
func width(s string) int { return utf8.RuneCountInString(s) }

// line joins spans into exactly w columns over an optional background, cutting
// with "…" when the text does not fit. Every span re-applies the background,
// because a style reset would clear it.
func line(w int, bg string, spans ...span) string {
	var b strings.Builder
	n := 0
	for _, s := range spans {
		if n >= w {
			break
		}
		t := s.text
		if n+width(t) > w {
			r := []rune(t)
			t = string(r[:max(w-n-1, 0)]) + "…"
		}
		b.WriteString(reset + bg + s.style + t)
		n += width(t)
	}
	if n < w {
		b.WriteString(reset + bg + strings.Repeat(" ", w-n))
	}
	b.WriteString(reset)
	return b.String()
}

// statusStyle is a status's symbol, color and word.
func statusStyle(s string) (icon, color, word string) {
	switch s {
	case status.Working:
		return "●", cWorking, "st.working"
	case status.Waiting:
		return "◐", cWaiting, "st.waiting"
	case status.Done:
		return "✓", cDone, "st.done"
	case status.Ended:
		return "✕", cEnded, "st.ended"
	default:
		return "○", cQuiet, "st.idle"
	}
}

// tmuxColor turns one of our escape colors into a tmux style (for the pane title).
func tmuxColor(esc string) string {
	switch esc {
	case cWorking:
		return "#e5c07b"
	case cWaiting:
		return "#d19aec"
	case cDone:
		return "#98c379"
	case cEnded:
		return "#e06c75"
	default:
		return "#6c717c"
	}
}

// spark draws values from 0 to 100 as block characters.
func spark(vals []float64) string {
	const blocks = "▁▂▃▄▅▆▇█"
	r := []rune(blocks)
	var b strings.Builder
	for _, v := range vals {
		i := int(v / 100 * float64(len(r)))
		b.WriteRune(r[min(max(i, 0), len(r)-1)])
	}
	return b.String()
}

func loadColor(cpu float64) string {
	switch {
	case cpu >= 80:
		return cEnded
	case cpu >= 50:
		return cWorking
	default:
		return cDone
	}
}

// isTemp reports folders that only hold throwaway sessions (tests, scratch).
func isTemp(dir string) bool {
	for _, pre := range []string{"/private/tmp/", "/tmp/", "/private/var/folders/", "/var/folders/"} {
		if strings.HasPrefix(dir+"/", pre) {
			return true
		}
	}
	return false
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes color codes, leaving only the visible text.
func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// memColor: memory gets yellow from 75% and red from 90% (macOS keeps memory
// busy on purpose, so the thresholds are higher than for CPU).
func memColor(pct float64) string {
	switch {
	case pct >= 90:
		return cEnded
	case pct >= 75:
		return cWorking
	default:
		return cDone
	}
}

// pushLast appends v and keeps the last sparkLen values.
func pushLast(vals []float64, v float64) []float64 {
	vals = append(vals, v)
	if len(vals) > sparkLen {
		vals = vals[len(vals)-sparkLen:]
	}
	return vals
}
