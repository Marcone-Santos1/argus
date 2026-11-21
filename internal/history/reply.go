package history

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

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
