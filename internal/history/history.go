// Package history lista as conversas antigas do Claude Code (transcrições
// .jsonl) e do opencode (banco sqlite), para retomá-las.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ferramentas.
const (
	Claude   = "claude"
	Opencode = "opencode"
)

// Entry é uma conversa que pode ser retomada.
type Entry struct {
	Tool    string    `json:"tool"`
	ID      string    `json:"id"`
	Dir     string    `json:"dir"`
	Title   string    `json:"title"`
	Account string    `json:"account"` // accountUuid (Claude)
	Updated time.Time `json:"updated"`
	Profile string    `json:"-"` // perfil sugerido; vazio = perguntar
}

// Key identifica a conversa entre ferramentas.
func (e Entry) Key() string { return e.Tool + ":" + e.ID }

// Loader sabe onde procurar conversas.
type Loader struct {
	ClaudeProjects  string            // ~/.claude/projects
	Cache           string            // índice para não reler transcrições sem mudança
	Accounts        map[string]string // accountUuid -> perfil
	OpencodeDB      string            // vazio = sem opencode
	OpencodeProfile string
	SQLite          string // binário do sqlite3 (padrão /usr/bin/sqlite3)

	mu sync.Mutex
}

type cacheItem struct {
	Size  int64 `json:"size"`
	MTime int64 `json:"mtime"`
	Entry Entry `json:"entry"`
}

// Chunk é quanto se lê do começo e do fim de cada transcrição.
const Chunk = 512 << 10

// Load devolve as conversas que ainda podem ser retomadas, mais recentes primeiro.
func (l *Loader) Load() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	cache := map[string]cacheItem{}
	if l.Cache != "" {
		if b, err := os.ReadFile(l.Cache); err == nil {
			_ = json.Unmarshal(b, &cache)
		}
	}
	fresh := map[string]cacheItem{}
	var out []Entry

	dirs, _ := os.ReadDir(l.ClaudeProjects)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(l.ClaudeProjects, d.Name()))
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(l.ClaudeProjects, d.Name(), f.Name())
			fi, err := f.Info()
			if err != nil || fi.Size() == 0 {
				continue
			}
			item, ok := cache[path]
			if !ok || item.Size != fi.Size() || item.MTime != fi.ModTime().UnixNano() {
				e, ok := ParseTranscript(path, fi.Size())
				if !ok {
					continue
				}
				e.Updated = fi.ModTime()
				item = cacheItem{Size: fi.Size(), MTime: fi.ModTime().UnixNano(), Entry: e}
			}
			fresh[path] = item
			out = append(out, item.Entry)
		}
	}
	if l.Cache != "" {
		if b, err := json.Marshal(fresh); err == nil {
			tmp := l.Cache + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				_ = os.Rename(tmp, l.Cache)
			}
		}
	}
	for i := range out {
		out[i].Profile = l.Accounts[out[i].Account]
		if out[i].Account == "" && len(l.Accounts) == 1 {
			for _, p := range l.Accounts {
				out[i].Profile = p
			}
		}
	}
	out = append(out, l.opencode()...)

	kept := out[:0]
	for _, e := range out {
		if e.Dir != "" && isDir(e.Dir) {
			kept = append(kept, e)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Updated.After(kept[j].Updated) })
	return kept
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// AccountOf lê a conta logada num .claude.json.
func AccountOf(claudeJSON string) string {
	b, err := os.ReadFile(claudeJSON)
	if err != nil {
		return ""
	}
	var c struct {
		OAuth struct {
			AccountUUID string `json:"accountUuid"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(b, &c) != nil {
		return ""
	}
	return c.OAuth.AccountUUID
}

var cwdRe = regexp.MustCompile(`"cwd":"((?:[^"\\]|\\.)*)"`)

func readAt(f *os.File, off, n int64) []byte {
	buf := make([]byte, n)
	k, _ := f.ReadAt(buf, off)
	return buf[:k]
}

// ParseTranscript lê só o começo (pasta, primeiro prompt) e o fim (título, conta)
// de uma transcrição do Claude Code. As transcrições podem ter centenas de MB.
func ParseTranscript(path string, size int64) (Entry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Entry{}, false
	}
	defer f.Close()
	e := Entry{Tool: Claude, ID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}

	head := readAt(f, 0, min(size, Chunk))
	tailOff := max(0, size-Chunk)
	tail := head
	if tailOff > 0 {
		tail = readAt(f, tailOff, size-tailOff)
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:] // primeira linha veio cortada
		}
	}
	if m := cwdRe.FindSubmatch(head); m != nil {
		var s string
		if json.Unmarshal([]byte(`"`+string(m[1])+`"`), &s) == nil {
			e.Dir = s
		}
	}
	scanMeta(tail, &e)
	if (e.Title == "" || e.Account == "") && tailOff > 0 {
		scanMeta(head, &e)
	}
	if e.Title == "" {
		e.Title = FirstPrompt(head)
	}
	return e, e.Dir != ""
}

// scanMeta procura, de trás para frente, o título mais recente e a conta.
func scanMeta(b []byte, e *Entry) {
	lines := bytes.Split(b, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if len(l) > 64<<10 {
			continue
		}
		switch {
		case e.Title == "" && bytes.Contains(l, []byte(`"type":"custom-title"`)):
			var r struct {
				CustomTitle string `json:"customTitle"`
				Title       string `json:"title"`
			}
			if json.Unmarshal(l, &r) == nil {
				e.Title = firstNonEmpty(r.CustomTitle, r.Title)
			}
		case e.Title == "" && bytes.Contains(l, []byte(`"type":"ai-title"`)):
			var r struct {
				AITitle string `json:"aiTitle"`
			}
			if json.Unmarshal(l, &r) == nil {
				e.Title = r.AITitle
			}
		case e.Account == "" && bytes.Contains(l, []byte(`"type":"session-owner"`)):
			var r struct {
				AccountUUID string `json:"accountUuid"`
			}
			if json.Unmarshal(l, &r) == nil {
				e.Account = r.AccountUUID
			}
		}
		if e.Title != "" && e.Account != "" {
			return
		}
	}
}

// FirstPrompt devolve o primeiro prompt digitado (sem mensagens internas).
func FirstPrompt(head []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(head))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := sc.Bytes()
		if !bytes.Contains(l, []byte(`"type":"user"`)) || bytes.Contains(l, []byte(`"isMeta":true`)) {
			continue
		}
		var r struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(l, &r) != nil {
			continue
		}
		var text string
		if json.Unmarshal(r.Message.Content, &text) != nil {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(r.Message.Content, &parts) == nil {
				for _, p := range parts {
					if p.Type == "text" {
						text = p.Text
						break
					}
				}
			}
		}
		text = strings.TrimSpace(text)
		if text == "" || strings.HasPrefix(text, "<") {
			continue
		}
		return strings.Join(strings.Fields(text), " ")
	}
	return "(sem título)"
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// opencode lê o banco do opencode com o sqlite3 do sistema (um processo, só ao abrir).
func (l *Loader) opencode() []Entry {
	if l.OpencodeDB == "" || l.OpencodeProfile == "" {
		return nil
	}
	if _, err := os.Stat(l.OpencodeDB); err != nil {
		return nil
	}
	bin := l.SQLite
	if bin == "" {
		bin = "/usr/bin/sqlite3"
	}
	q := `select id, directory, title, time_updated from session_v2 where parent_id is null and time_archived is null
union all select id, directory, title, time_updated from session where parent_id is null and time_archived is null
  and id not in (select id from session_v2);`
	cmd := exec.Command(bin, "-readonly", "-separator", "\x1f", "file:"+l.OpencodeDB+"?mode=ro", q)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParseOpencodeRows(string(out), l.OpencodeProfile)
}

// ParseOpencodeRows interpreta a saída do sqlite3 (campos separados por 0x1f).
func ParseOpencodeRows(out, profile string) []Entry {
	var es []Entry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		ms, _ := strconv.ParseInt(f[3], 10, 64)
		es = append(es, Entry{Tool: Opencode, ID: f[0], Dir: f[1], Title: f[2], Updated: time.UnixMilli(ms), Profile: profile})
	}
	return es
}

// ResumeArgs são os argumentos que cada ferramenta usa para retomar uma conversa.
func ResumeArgs(tool, id string) []string {
	if tool == Opencode {
		return []string{"--session", id}
	}
	return []string{"--resume", id}
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// SafeID diz se o id pode ir numa linha de comando sem escape.
func SafeID(id string) bool { return safeID.MatchString(id) }
