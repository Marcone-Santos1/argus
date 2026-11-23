package history

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// TranscriptPath finds a Claude conversation's transcript ("" if none).
func TranscriptPath(projects, id string) string {
	if !SafeID(id) {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(projects, "*", id+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// LastReply is the start of the last text Claude wrote in a conversation,
// read from the end of its transcript: the first line, without markdown.
func LastReply(projects, id string) string {
	path := TranscriptPath(projects, id)
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := max(0, fi.Size()-Chunk)
	tail := readAt(f, off, fi.Size()-off)
	return lastReply(tail)
}

func lastReply(tail []byte) string {
	lines := bytes.Split(tail, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(`"assistant"`)) || !gjson.ValidBytes(l) {
			continue
		}
		js := gjson.ParseBytes(l)
		if js.Get("type").String() != "assistant" {
			continue
		}
		content := js.Get("message.content").Array()
		for j := len(content) - 1; j >= 0; j-- {
			if content[j].Get("type").String() == "text" {
				if s := firstLine(content[j].Get("text").String()); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// firstLine is the first line with text, without markdown marks.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimLeft(l, "#>-*• ")
		l = strings.NewReplacer("**", "", "`", "", "__", "").Replace(l)
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// agentStale is how long a background agent's transcript may go unwritten
// before it is taken for gone (its conversation was closed mid-run).
const agentStale = 30 * time.Minute

// BackgroundAgents counts a conversation's background subagents still
// running, from the small meta file Claude Code keeps next to each one:
// "finished" is set when the agent ends. One stat per agent, and a read only
// for the recent ones.
func BackgroundAgents(projects, id string, now time.Time) int {
	path := TranscriptPath(projects, id)
	if path == "" {
		return 0
	}
	metas, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.meta.json"))
	n := 0
	for _, m := range metas {
		fi, err := os.Stat(strings.TrimSuffix(m, ".meta.json") + ".jsonl")
		if err != nil || now.Sub(fi.ModTime()) > agentStale {
			continue
		}
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if gjson.GetBytes(data, "requestShape").String() == "background" && !gjson.GetBytes(data, "finished").Bool() {
			n++
		}
	}
	return n
}
