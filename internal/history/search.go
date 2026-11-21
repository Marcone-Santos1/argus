package history

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// Hit is a conversation whose text contains the searched words.
type Hit struct {
	Key     string // Entry.Key()
	Snippet string // the text around the first match
}

const searchChunk = 4 << 20

// Search looks for the text inside every conversation (case-insensitive):
// Claude transcripts are read in parallel, opencode's database with one
// sqlite3 query. It stops early when ctx is cancelled (a new search).
func (l *Loader) Search(ctx context.Context, query string) []Hit {
	q := bytes.ToLower([]byte(strings.TrimSpace(query)))
	if len(q) < 2 {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(l.ClaudeProjects, "*", "*.jsonl"))
	jobs := make(chan string)
	var mu sync.Mutex
	var hits []Hit
	var wg sync.WaitGroup
	for range max(runtime.NumCPU()-1, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, searchChunk+len(q))
			for f := range jobs {
				if snip, ok := searchFile(f, q, buf); ok {
					mu.Lock()
					hits = append(hits, Hit{Key: Claude + ":" + strings.TrimSuffix(filepath.Base(f), ".jsonl"), Snippet: snip})
					mu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		select {
		case jobs <- f:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return append(hits, l.searchOpencode(ctx, string(q))...)
}

// searchFile reads the file in chunks (overlapping by the query's length,
// so a match across two chunks is found) and returns the first match inside
// what the person or Claude wrote; matches in paths, tool listings or the
// system prompt are skipped.
func searchFile(path string, q, buf []byte) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	keep := 0
	for {
		n, err := io.ReadFull(f, buf[keep:])
		data := buf[:keep+n]
		lower := bytes.ToLower(data)
		for from := 0; ; {
			i := bytes.Index(lower[from:], q)
			if i < 0 {
				break
			}
			i += from
			start := bytes.LastIndexByte(data[:i], '\n') + 1
			end := bytes.IndexByte(data[i:], '\n')
			if end < 0 {
				end = len(data)
			} else {
				end += i
			}
			if text := messageText(data[start:end]); text != "" {
				if j := strings.Index(strings.ToLower(text), string(q)); j >= 0 {
					return snippet([]byte(text), j, len(q)), true
				}
			}
			from = end
			if from >= len(data) {
				break
			}
		}
		if err != nil {
			return "", false
		}
		keep = min(len(q), len(data))
		copy(buf, data[len(data)-keep:])
	}
}

// messageText is the text of a prompt or a reply in a transcript line
// ("" for anything else: tool results, attachments, metadata).
func messageText(line []byte) string {
	if !gjson.ValidBytes(line) {
		return ""
	}
	js := gjson.ParseBytes(line)
	t := js.Get("type").String()
	if (t != "user" && t != "assistant") || js.Get("isMeta").Bool() {
		return ""
	}
	c := js.Get("message.content")
	if c.Type == gjson.String {
		return c.String()
	}
	var b strings.Builder
	for _, part := range c.Array() {
		if part.Get("type").String() == "text" {
			b.WriteString(part.Get("text").String())
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// snippet is the readable text around a match: JSON escapes and quotes out,
// cut on whole characters.
func snippet(data []byte, at, n int) string {
	from, to := max(0, at-50), min(len(data), at+n+70)
	for from > 0 && !utf8.RuneStart(data[from]) {
		from++
	}
	for to < len(data) && !utf8.RuneStart(data[to]) {
		to--
	}
	s := string(data[from:to])
	s = strings.NewReplacer(`\n`, " ", `\t`, " ", `\"`, `"`, `\\`, `\`, "\n", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// searchOpencode finds opencode conversations whose parts contain q.
func (l *Loader) searchOpencode(ctx context.Context, q string) []Hit {
	if l.OpencodeDB == "" {
		return nil
	}
	if _, err := os.Stat(l.OpencodeDB); err != nil {
		return nil
	}
	bin := l.SQLite
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("sqlite3"); err != nil {
			return nil
		}
	}
	lit := "'" + strings.ReplaceAll(q, "'", "''") + "'"
	like := "'%" + strings.NewReplacer("'", "''", `\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%' escape '\\'"
	sel := func(table string) string {
		return "select session_id as sid, substr(data, max(1, instr(lower(data), " + lit + ") - 50), 130) as s from " +
			table + " where lower(data) like " + like
	}
	query := "select sid, min(s) from (" + sel("part") + " union all " + sel("session_part") + ") group by sid;"
	cmd := exec.CommandContext(ctx, bin, "-readonly", "-separator", "\x1f", "file:"+l.OpencodeDB+"?mode=ro", query)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var hits []Hit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		id, snip, ok := strings.Cut(line, "\x1f")
		if ok && SafeID(id) {
			hits = append(hits, Hit{Key: Opencode + ":" + id, Snippet: snippet([]byte(snip), 0, 0)})
		}
	}
	return hits
}
