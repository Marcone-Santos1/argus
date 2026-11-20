package main

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

// Histórico: conversas antigas do Claude Code (transcrições .jsonl) e do
// opencode (banco sqlite), lidas só quando o usuário abre o histórico.

type histEntry struct {
	Tool    string    `json:"tool"` // claude | opencode
	ID      string    `json:"id"`
	Dir     string    `json:"dir"`
	Title   string    `json:"title"`
	Account string    `json:"account"` // accountUuid (claude)
	Updated time.Time `json:"updated"`
	Profile string    `json:"-"` // perfil sugerido; vazio = perguntar
}

type histCacheItem struct {
	Size  int64     `json:"size"`
	MTime int64     `json:"mtime"`
	Entry histEntry `json:"entry"`
}

func histCachePath() string { return filepath.Join(stateDir, "_history.json") }

const chunk = 512 << 10

var cwdRe = regexp.MustCompile(`"cwd":"((?:[^"\\]|\\.)*)"`)

// loadHistory devolve as conversas mais recentes primeiro.
var historyMu sync.Mutex

func loadHistory(cfg *Config) []histEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	accounts := accountProfiles(cfg)
	cache := map[string]histCacheItem{}
	if b, err := os.ReadFile(histCachePath()); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	fresh := map[string]histCacheItem{}
	var out []histEntry

	root := filepath.Join(homeDir(), ".claude", "projects")
	dirs, _ := os.ReadDir(root)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(root, d.Name()))
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(root, d.Name(), f.Name())
			fi, err := f.Info()
			if err != nil || fi.Size() == 0 {
				continue
			}
			item, ok := cache[path]
			if !ok || item.Size != fi.Size() || item.MTime != fi.ModTime().UnixNano() {
				e, ok := parseTranscript(path, fi.Size())
				if !ok {
					continue
				}
				e.Updated = fi.ModTime()
				item = histCacheItem{Size: fi.Size(), MTime: fi.ModTime().UnixNano(), Entry: e}
			}
			fresh[path] = item
			out = append(out, item.Entry)
		}
	}
	if b, err := json.Marshal(fresh); err == nil {
		tmp := histCachePath() + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			_ = os.Rename(tmp, histCachePath())
		}
	}
	for i := range out {
		out[i].Profile = accounts[out[i].Account]
		if out[i].Account == "" && len(accounts) == 1 {
			for _, p := range accounts {
				out[i].Profile = p
			}
		}
	}

	out = append(out, opencodeSessions(cfg)...)

	// Só o que ainda pode ser retomado: a pasta precisa existir.
	kept := out[:0]
	for _, e := range out {
		if e.Dir != "" && exists(e.Dir) {
			kept = append(kept, e)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Updated.After(kept[j].Updated) })
	return kept
}

// accountProfiles mapeia accountUuid -> perfil, lendo o .claude.json de cada conta.
func accountProfiles(cfg *Config) map[string]string {
	m := map[string]string{}
	for _, p := range cfg.Profiles {
		if p.Label() != "claude" {
			continue
		}
		file := filepath.Join(homeDir(), ".claude.json")
		if v, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
			file = filepath.Join(expand(v), ".claude.json")
		}
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var c struct {
			OAuth struct {
				AccountUUID string `json:"accountUuid"`
			} `json:"oauthAccount"`
		}
		if json.Unmarshal(b, &c) == nil && c.OAuth.AccountUUID != "" {
			if _, dup := m[c.OAuth.AccountUUID]; !dup {
				m[c.OAuth.AccountUUID] = p.Name
			}
		}
	}
	return m
}

func readAt(f *os.File, off, n int64) []byte {
	buf := make([]byte, n)
	k, _ := f.ReadAt(buf, off)
	return buf[:k]
}

// parseTranscript lê só o começo (pasta, primeiro prompt) e o fim (título, conta).
func parseTranscript(path string, size int64) (histEntry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return histEntry{}, false
	}
	defer f.Close()
	e := histEntry{Tool: "claude", ID: strings.TrimSuffix(filepath.Base(path), ".jsonl")}

	head := readAt(f, 0, min(size, chunk))
	tailOff := max(0, size-chunk)
	tail := head
	if tailOff > 0 {
		tail = readAt(f, tailOff, size-tailOff)
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:] // primeira linha cortada
		}
	}

	if m := cwdRe.FindSubmatch(head); m != nil {
		var s string
		if json.Unmarshal([]byte(`"`+string(m[1])+`"`), &s) == nil {
			e.Dir = s
		}
	}
	scanMeta := func(b []byte) {
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
	scanMeta(tail)
	if (e.Title == "" || e.Account == "") && tailOff > 0 {
		scanMeta(head)
	}
	if e.Title == "" {
		e.Title = firstPrompt(head)
	}
	if e.Dir == "" {
		return e, false
	}
	return e, true
}

func firstPrompt(head []byte) string {
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

// opencodeSessions lê o banco do opencode com o sqlite3 do sistema (um processo, só ao abrir).
func opencodeSessions(cfg *Config) []histEntry {
	prof := ""
	for _, p := range cfg.Profiles {
		if p.Label() == "opencode" {
			prof = p.Name
			break
		}
	}
	db := filepath.Join(homeDir(), ".local", "share", "opencode", "opencode.db")
	if prof == "" || !exists(db) {
		return nil
	}
	q := `select id, directory, title, time_updated from session_v2 where parent_id is null and time_archived is null
union all select id, directory, title, time_updated from session where parent_id is null and time_archived is null
  and id not in (select id from session_v2);`
	cmd := exec.Command("/usr/bin/sqlite3", "-readonly", "-separator", "\x1f", "file:"+db+"?mode=ro", q)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var es []histEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		ms, _ := strconv.ParseInt(f[3], 10, 64)
		es = append(es, histEntry{Tool: "opencode", ID: f[0], Dir: f[1], Title: f[2], Updated: time.UnixMilli(ms), Profile: prof})
	}
	return es
}

// resumeArgs: como cada ferramenta retoma uma conversa.
func resumeArgs(tool, id string) string {
	switch tool {
	case "opencode":
		return " --session " + id
	default:
		return " --resume " + id
	}
}
