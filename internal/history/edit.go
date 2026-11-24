package history

import (
	"bytes"
	"os"
	"strings"

	"github.com/tidwall/gjson"
)

// Edit is a file change the agent made: where, and the first changed line.
type Edit struct {
	File string
	Line int
	Key  string // the transcript entry, to tell one edit from the next
}

// LastEdit is the last file Claude edited or wrote in a conversation, read
// from the end of its transcript.
func LastEdit(path string) (Edit, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Edit{}, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Edit{}, false
	}
	off := max(0, fi.Size()-Chunk)
	return lastEdit(readAt(f, off, fi.Size()-off))
}

func lastEdit(tail []byte) (Edit, bool) {
	lines := bytes.Split(tail, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if !bytes.Contains(l, []byte(`"filePath"`)) || !gjson.ValidBytes(l) {
			continue
		}
		r := gjson.GetBytes(l, "toolUseResult")
		file := r.Get("filePath").String()
		if file == "" || !r.Get("structuredPatch").Exists() {
			continue // a read, not a change
		}
		e := Edit{File: file, Line: 1, Key: gjson.GetBytes(l, "uuid").String()}
		if hunk := r.Get("structuredPatch.0"); hunk.Exists() {
			e.Line = int(hunk.Get("newStart").Int())
			// The hunk starts with context lines; the change is after them.
			for _, hl := range hunk.Get("lines").Array() {
				s := hl.String()
				if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
					break
				}
				e.Line++
			}
			e.Line = max(e.Line, 1)
		}
		return e, true
	}
	return Edit{}, false
}
