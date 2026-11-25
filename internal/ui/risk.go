package ui

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Marcone-Santos1/argus/internal/i18n"
)

// ---------- risky prompts: what deserves a second look before y ----------
//
// A session waiting for you shows what it asks under it, and y answers from
// the list. With a dozen sessions it is easy to say yes by reflex, so a
// prompt that deletes, overwrites, force-pushes, runs as root or writes
// outside the session's folder is marked, and Enter alone does not accept it.

// riskRule is one kind of risk: a pattern over the command and its label.
type riskRule struct {
	key string
	re  *regexp.Regexp
}

var riskRules = []riskRule{
	{"risk.delete", regexp.MustCompile(`(?m)(^|[\s;&|(])rm\s+(-[a-zA-Z]*[rR][a-zA-Z]*|--recursive)\b`)},
	{"risk.sudo", regexp.MustCompile(`(?m)(^|[\s;&|(])sudo\s`)},
	{"risk.forcePush", regexp.MustCompile(`git\s+push\b[^\n;&|]*\s(--force\b|--force-with-lease\b|-f\b|\+\S)`)},
	{"risk.discard", regexp.MustCompile(`git\s+(reset\s+[^\n;&|]*--hard|clean\s+-[a-zA-Z]*f|checkout\s+(--\s+)?\.(\s|$)|restore\s+[^\n;&|]*\.(\s|$)|branch\s+-D\b|stash\s+(drop|clear)\b)`)},
	{"risk.pipeShell", regexp.MustCompile(`(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`)},
	{"risk.perms", regexp.MustCompile(`(?m)(^|[\s;&|(])(chmod|chown)\s+(-[a-zA-Z]*R|[0-7]*777)`)},
	{"risk.disk", regexp.MustCompile(`(?m)(^|[\s;&|(])(dd\s+[^\n]*of=|mkfs|diskutil\s+(erase|partition))`)},
	{"risk.data", regexp.MustCompile(`(?i)\b(drop\s+(table|database|schema)|truncate\s+table|delete\s+from\s+\S+\s*(;|"|'|$))`)},
	{"risk.kill", regexp.MustCompile(`(?m)(^|[\s;&|(])(kill\s+-9|killall|pkill)\b`)},
	{"risk.docker", regexp.MustCompile(`docker\s+(system\s+prune|volume\s+(rm|prune)|compose\s+down\s+[^\n]*-v\b)`)},
}

// writeRe finds commands that change files; their absolute paths are
// checked against the session's folder.
var (
	writeRe = regexp.MustCompile(`(?m)(^|[\s;&|(])(rm|mv|cp|tee|touch|mkdir|ln|truncate|rsync|chmod|chown|sed\s+-i)\s|>>?\s*[~/]`)
	pathRe  = regexp.MustCompile(`(?:^|[\s'"=>])((?:~|/)[^\s'"&|;)]*)`)
)

// safeDirs are outside the project but harmless to write to.
var safeDirs = []string{"/tmp", "/private/tmp", "/var/folders", "/private/var/folders", "/dev/null", "/dev/stdout", "/dev/stderr"}

// riskOf lists what is risky in a prompt, as short labels ("" none). dir
// is the session's folder, home expands "~".
func riskOf(pr prompt, dir, home string) []string {
	text := pr.Body
	if text == "" {
		text = pr.Context
	}
	var out []string
	for _, r := range riskRules {
		if r.re.MatchString(text) {
			out = append(out, i18n.T(r.key))
		}
	}
	// A file edit asks about its file in the question; a command writes
	// where its paths point.
	var places []string
	if strings.Contains(strings.ToLower(pr.Question), "edit") || strings.Contains(strings.ToLower(pr.Question), "create") || strings.Contains(strings.ToLower(pr.Question), "write") {
		places = append(places, pr.Question, pr.Context)
	}
	for _, l := range strings.Split(text, "\n") {
		if writeRe.MatchString(l) {
			places = append(places, l)
		}
	}
	if p := outsidePath(places, dir, home); p != "" {
		out = append(out, i18n.T("risk.outside", p))
	}
	return out
}

// outsidePath is the first absolute path in the texts that is not inside
// dir (nor a scratch folder).
func outsidePath(texts []string, dir, home string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	for _, t := range texts {
		for _, m := range pathRe.FindAllStringSubmatch(t, -1) {
			p := strings.TrimRight(m[1], ".,:?")
			if p == "/" || p == "~" {
				return p
			}
			if strings.HasPrefix(p, "~/") {
				p = filepath.Join(home, p[2:])
			}
			p = filepath.Clean(p)
			if p == dir || strings.HasPrefix(p, dir+"/") || underAny(p, safeDirs) {
				continue
			}
			return shortPath(p, home)
		}
	}
	return ""
}

func underAny(p string, dirs []string) bool {
	for _, d := range dirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

func shortPath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}
