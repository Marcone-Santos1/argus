// Package usage reads what Claude Code passes to its status line: the
// account's rate limits (5 hours, 7 days) and how full the context is. The
// Argus status line script saves that JSON per session.
package usage

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Limit is one rate-limit window.
type Limit struct {
	Percent float64   // 0-100
	Resets  time.Time // zero when unknown
}

// Info is one session's last status-line report.
type Info struct {
	Session  string
	FiveHour *Limit
	SevenDay *Limit
	Context  float64 // % of the context window in use; -1 = unknown
	Model    string  // short model name: "opus-5.5"
	At       time.Time
}

// Read parses a saved report.
func Read(path string) (Info, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return Info{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, false
	}
	return Parse(data, fi.ModTime())
}

// Parse interprets the status-line JSON.
func Parse(data []byte, at time.Time) (Info, bool) {
	if !gjson.ValidBytes(data) {
		return Info{}, false
	}
	js := gjson.ParseBytes(data)
	in := Info{Session: js.Get("session_id").String(), Context: -1, At: at}
	in.FiveHour = limit(js.Get("rate_limits.five_hour"))
	in.SevenDay = limit(js.Get("rate_limits.seven_day"))
	in.Model = ShortModel(js.Get("model.display_name").String())
	if c := js.Get("context_window.used_percentage"); c.Exists() {
		in.Context = c.Float()
	}
	return in, true
}

func limit(v gjson.Result) *Limit {
	pct := v.Get("used_percentage")
	if !pct.Exists() {
		return nil
	}
	l := &Limit{Percent: pct.Float()}
	r := v.Get("resets_at")
	switch r.Type {
	case gjson.Number:
		n := r.Int()
		if n > 1e12 { // milliseconds
			n /= 1000
		}
		if n > 0 {
			l.Resets = time.Unix(n, 0)
		}
	case gjson.String:
		if t, err := time.Parse(time.RFC3339, r.String()); err == nil {
			l.Resets = t
		} else if n, err := strconv.ParseInt(r.String(), 10, 64); err == nil {
			l.Resets = time.Unix(n, 0)
		}
	}
	return l
}

// Expired reports whether the window has already reset (the numbers no
// longer hold).
func (l *Limit) Expired(now time.Time) bool {
	return l == nil || (!l.Resets.IsZero() && now.After(l.Resets))
}

// ShortModel turns a display name into a compact one: "Opus 5.5 (1M
// context)" becomes "opus-5.5".
func ShortModel(name string) string {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}
