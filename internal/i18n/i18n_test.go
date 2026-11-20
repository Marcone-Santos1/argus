package i18n

import (
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		m    map[string]string
		want Lang
	}{
		{map[string]string{"LANG": "pt_BR.UTF-8"}, PT},
		{map[string]string{"LANG": "en_US.UTF-8"}, EN},
		{map[string]string{"LANG": "pt_BR.UTF-8", "LC_ALL": "en_US.UTF-8"}, EN},
		{map[string]string{"LANG": "en_US.UTF-8", "ARGUS_LANG": "pt"}, PT},
		{map[string]string{"LC_ALL": "C", "LANG": "pt_PT"}, PT},
		{map[string]string{}, EN},
		{map[string]string{"LANG": "de_DE"}, EN},
	}
	for _, c := range cases {
		if got := Detect(env(c.m)); got != c.want {
			t.Errorf("Detect(%v) = %v, want %v", c.m, got, c.want)
		}
	}
}

// Every message has an English text, and Portuguese texts keep the same
// formatting verbs, so a translation can never break a Sprintf.
func TestCatalog(t *testing.T) {
	verbs := func(s string) string {
		var out []string
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '%' {
				j := i + 1
				for j < len(s) && strings.ContainsRune("0123456789.-+ #", rune(s[j])) {
					j++
				}
				if j < len(s) {
					out = append(out, string(s[j]))
					i = j
				}
			}
		}
		return strings.Join(out, ",")
	}
	for k, m := range messages {
		if m[EN] == "" {
			t.Errorf("%s: no English text", k)
		}
		if m[PT] != "" && verbs(m[EN]) != verbs(m[PT]) {
			t.Errorf("%s: verbs differ: %q vs %q", k, verbs(m[EN]), verbs(m[PT]))
		}
	}
}

func TestT(t *testing.T) {
	defer Set(Current())
	Set(PT)
	if got := T("list.sessions", 3); got != "3 sessões" {
		t.Errorf("PT: %q", got)
	}
	Set(EN)
	if got := T("list.sessions", 3); got != "3 sessions" {
		t.Errorf("EN: %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key: %q", got)
	}
	if !Yes("y") || !Yes("s") || !Yes("enter") || Yes("n") {
		t.Error("Yes")
	}
}
