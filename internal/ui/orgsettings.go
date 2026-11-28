package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/i18n"
)

// A Claude Team or Enterprise owner can push settings to Claude Code: they
// land in remote-settings.json in the account's config folder and can, for
// instance, turn on telemetry that records prompts. Argus tells you when that
// file changes, so you know when your organization starts enforcing or
// collecting something.

const orgCheckEvery = time.Minute

// orgFile is one account's organization settings file.
type orgFile struct {
	Profile, Path string
}

// orgFiles lists the remote-settings.json of each Claude account, once per
// config folder.
func (p *Panel) orgFiles() []orgFile {
	var out []orgFile
	seen := map[string]bool{}
	for _, pr := range p.d.Config.Profiles {
		if pr.Label() != history.Claude {
			continue
		}
		dir := pr.ClaudeConfigDir(p.d.Paths.Home)
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, orgFile{pr.Name, filepath.Join(dir, "remote-settings.json")})
	}
	return out
}

// orgKeys names what the organization set: its top-level settings, and each
// variable inside "env" as env.NAME. Empty means nothing is set ({} or no
// file).
func orgKeys(data []byte) []string {
	var m map[string]json.RawMessage
	if len(strings.TrimSpace(string(data))) == 0 || json.Unmarshal(data, &m) != nil {
		return nil
	}
	var keys []string
	for k, v := range m {
		if k == "env" {
			var env map[string]json.RawMessage
			if json.Unmarshal(v, &env) == nil {
				for e := range env {
					keys = append(keys, "env."+e)
				}
				continue
			}
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *Panel) orgSeenPath() string { return filepath.Join(p.d.Paths.State, "_org.json") }

// orgCheck warns (in the panel and as a notification) when an account's
// organization settings changed since Argus last looked, and on the first
// look when something is already set.
func (p *Panel) orgCheck(now time.Time) {
	if now.Sub(p.orgCheckedAt) < orgCheckEvery {
		return
	}
	p.orgCheckedAt = now
	seen := map[string]string{}
	if data, err := os.ReadFile(p.orgSeenPath()); err == nil {
		_ = json.Unmarshal(data, &seen)
	}
	changed := false
	for _, f := range p.orgFiles() {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			data = nil
		}
		keys := orgKeys(data)
		sum := sha256.Sum256([]byte(strings.Join(keys, "\n") + "\x00" + string(data)))
		h := hex.EncodeToString(sum[:])
		if len(keys) == 0 {
			h = "" // {} and no file are the same: nothing set
		}
		old, known := seen[f.Path]
		if known && old == h || !known && h == "" {
			seen[f.Path] = h
			continue
		}
		seen[f.Path], changed = h, true
		var msg string
		if h == "" {
			msg = i18n.T("msg.orgCleared", f.Profile)
		} else {
			msg = i18n.T("msg.orgChanged", f.Profile, strings.Join(keys, ", "))
		}
		p.flash(msg)
		p.msgUntil = now.Add(time.Minute)
		p.d.Notify(i18n.T("notify.orgTitle"), msg)
	}
	if changed {
		if data, err := json.Marshal(seen); err == nil {
			_ = os.WriteFile(p.orgSeenPath(), data, 0o644)
		}
	}
}
