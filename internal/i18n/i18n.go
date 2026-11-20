// Package i18n translates user-facing text. The code and its comments are in
// English; the screen follows the system language (Portuguese or English).
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang is a supported language.
type Lang int

const (
	EN Lang = iota
	PT
)

var current = Detect(os.Getenv)

// Detect picks the language from ARGUS_LANG, then the usual locale variables.
// Anything that is not Portuguese falls back to English.
func Detect(getenv func(string) string) Lang {
	for _, k := range []string{"ARGUS_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.ToLower(getenv(k)); v != "" && v != "c" && v != "posix" {
			if strings.HasPrefix(v, "pt") {
				return PT
			}
			return EN
		}
	}
	return EN
}

// Set changes the language (used by tests).
func Set(l Lang) { current = l }

// Current returns the active language.
func Current() Lang { return current }

// T returns the message for key in the active language, formatted with args.
// An unknown key is returned as is, so a missing translation is visible.
func T(key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s := m[current]
	if s == "" {
		s = m[EN]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Yes reports whether a key answers "yes" to a confirmation (y, s or Enter).
func Yes(k string) bool {
	switch k {
	case "y", "Y", "s", "S", "enter":
		return true
	}
	return false
}
