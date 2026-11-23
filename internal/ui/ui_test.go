package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Marcone-Santos1/argus/internal/config"
	"github.com/Marcone-Santos1/argus/internal/history"
	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

func TestParseKeys(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"\x1b[A\x1b[B", []string{"up", "down"}},
		{"\x1bOA", []string{"up"}},
		{"\x1b", []string{"esc"}},
		{"\r\x7f\t", []string{"enter", "backspace", "tab"}},
		{"\x15", []string{"ctrl-U"}},
		{"aç", []string{"a", "ç"}},
		{"\x1b[Z", []string{"shift-tab"}},
		{"\x1b[1;5C", nil}, // arrow with a modifier: ignored
	}
	for _, c := range cases {
		if got := parseKeys([]byte(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseKeys(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFit(t *testing.T) {
	if got := fit("abc", 5); got != "abc  " {
		t.Errorf("pads: %q", got)
	}
	if got := fit("abcdef", 4); got != "abc…" {
		t.Errorf("cuts: %q", got)
	}
	if got := fit("ação", 4); got != "ação" {
		t.Errorf("counts runes, not bytes: %q", got)
	}
	if fit("x", 0) != "" {
		t.Error("width 0")
	}
}

func TestLayoutKeepsFocusVisible(t *testing.T) {
	var body []string
	for i := 0; i < 50; i++ {
		body = append(body, string(rune('A'+i%26)))
	}
	out := layout(body, []string{"footer"}, 11, 40)
	if len(out) != 11 || out[10] != "footer" {
		t.Fatalf("height and footer: %q", out)
	}
	if !strings.Contains(strings.Join(out[:10], ""), body[40]) {
		t.Fatal("the focus line must be visible")
	}
	short := layout([]string{"a"}, []string{"f"}, 4, 0)
	if !reflect.DeepEqual(short, []string{"a", "", "", "f"}) {
		t.Fatalf("pads with empty lines: %q", short)
	}
}

func TestWrapHints(t *testing.T) {
	lines := wrapHints([]string{"aaa", "bbb", "ccc"}, 10)
	if len(lines) != 2 {
		t.Fatalf("wraps into two lines: %q", lines)
	}
}

func testPanel(t *testing.T) *Panel {
	t.Helper()
	home := t.TempDir()
	cfg, err := config.Parse(`
[[profile]]
name = "personal"
group = "PERSONAL"
command = "claude"
[[profile]]
name = "work"
group = "WORK"
command = "claude"
[[profile]]
name = "opencode"
command = "opencode"
`, home)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Paths: paths.For(home, "x"), Config: cfg, Tmux: tmux.New("argus-ui-test-missing")})
}

func TestRebuildItemsGroupsByFolder(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Profile: "personal", Name: "api"},
		{ID: "a2", Dir: "/p/site", Profile: "opencode", Name: "site", Session: "ses_aberta"},
		{ID: "a3", Dir: "/p/api", Profile: "work", Name: "api 2"},
	}
	p.histAll = []history.Entry{
		{Tool: "claude", ID: "c1", Dir: "/p/docs", Title: "Docs", Updated: now},
		{Tool: "opencode", ID: "ses_aberta", Dir: "/p/site", Title: "Already open", Updated: now},
		{Tool: "claude", ID: "c2", Dir: "/p/api", Title: "Old api", Updated: now.Add(-time.Hour)},
		{Tool: "claude", ID: "c3", Dir: "/p/api", Title: "Api 3", Updated: now.Add(-2 * time.Hour)},
		{Tool: "claude", ID: "c4", Dir: "/p/api", Title: "Api 4", Updated: now.Add(-3 * time.Hour)},
		{Tool: "claude", ID: "c5", Dir: "/p/api", Title: "Api 5 (over the limit)", Updated: now.Add(-4 * time.Hour)},
	}
	p.rebuildItems("")
	list := func() string {
		var got []string
		for _, it := range p.items {
			switch {
			case it.agent >= 0:
				got = append(got, "session "+p.agents[it.agent].ID)
			case it.folder:
				got = append(got, fmt.Sprintf("folder %s %d", it.dir, it.count))
			default:
				got = append(got, "  conversation "+it.entry.ID)
			}
		}
		return strings.Join(got, "\n")
	}
	// Sessions first, grouped by folder; then one row per folder, newest
	// first; the conversation already open is not repeated.
	want := "session a1\nsession a3\nsession a2\nfolder /p/docs 1\nfolder /p/api 4"
	if got := list(); got != want {
		t.Fatalf("list:\n%s\nwant:\n%s", got, want)
	}
	if i := p.nthAgentItem(2); i != 2 {
		t.Errorf("key 3 goes to the third session: %d", i)
	}

	// Enter opens a folder: its conversations (up to the limit) show under it.
	p.selectKey("f:/p/api")
	p.handleKey("enter")
	want += "\n  conversation c2\n  conversation c3\n  conversation c4\n  conversation c5"
	if got := list(); got != want {
		t.Fatalf("open folder:\n%s", got)
	}
	// Left on a conversation closes its folder and selects it.
	p.handleKey("down")
	p.handleKey("left")
	if p.selKey() != "f:/p/api" || len(p.items) != 5 {
		t.Fatalf("left closes the folder: %s %d", p.selKey(), len(p.items))
	}

	// The selection follows the item, not the position.
	p.sel = 2
	key := p.selKey()
	p.agents = p.agents[1:] // a1 closed
	p.rebuildItems(key)
	if p.selKey() != key {
		t.Fatalf("selection lost: %s -> %s", key, p.selKey())
	}

	// A filter shows the matching conversations under their folders.
	p.listFilter = "docs"
	p.rebuildItems("")
	if len(p.items) != 2 || !p.items[0].folder || p.items[1].entry.ID != "c1" {
		t.Fatalf("filter: %+v", p.items)
	}
}

func TestOpencodeMatching(t *testing.T) {
	p := testPanel(t)
	created := time.Now().Add(-time.Minute)
	p.agents = []agent{
		{ID: "o1", Pane: "%1", Dir: "/p/api", Profile: "opencode", Created: created.UnixNano()},
		{ID: "o2", Pane: "%2", Dir: "/p/api", Profile: "opencode", Created: created.UnixNano(), Session: "ses_retomada"},
	}
	p.oc = map[string]status.Opencode{
		"ses_velha":    {Session: "ses_velha", Status: status.Done, Dir: "/p/api", Changed: created.Add(-time.Hour)},
		"ses_nova":     {Session: "ses_nova", Status: status.Working, Dir: "/p/api", Changed: time.Now()},
		"ses_retomada": {Session: "ses_retomada", Status: status.Waiting, Dir: "/p/api", Changed: time.Now()},
		"ses_outra":    {Session: "ses_outra", Status: status.Working, Dir: "/p/site", Changed: time.Now()},
	}
	oc, ok := p.opencodeFor(&p.agents[0])
	if !ok || oc.Session != "ses_nova" || p.agents[0].Session != "ses_nova" {
		t.Fatalf("a new session takes the newest conversation of its folder created after it: %+v", oc)
	}
	oc, ok = p.opencodeFor(&p.agents[1])
	if !ok || oc.Status != status.Waiting {
		t.Fatalf("a resumed session uses the id: %+v", oc)
	}
}

func TestKeyFilterAndInput(t *testing.T) {
	p := testPanel(t)
	p.histAll = []history.Entry{{Tool: "claude", ID: "c1", Dir: "/p/docs", Title: "Docs"}, {Tool: "claude", ID: "c2", Dir: "/p/api", Title: "Api"}}
	p.rebuildItems("")
	p.handleKey("/")
	for _, k := range []string{"a", "p", "i"} {
		p.handleKey(k)
	}
	if p.listFilter != "api" || len(p.items) != 2 {
		t.Fatalf("typed filter: %q %d", p.listFilter, len(p.items))
	}
	p.handleKey("enter")
	if p.mode != modeList || p.listFilter != "api" {
		t.Fatal("Enter leaves the filter and keeps it")
	}
	p.handleKey("esc")
	if p.listFilter != "" || len(p.items) != 2 {
		t.Fatal("Esc in the list clears the filter")
	}

	got := ""
	p.ask("name:", "abc", func(s string) { got = s })
	p.handleKey("backspace")
	p.handleKey("Z")
	p.handleKey("enter")
	if got != "abZ" || p.mode != modeList {
		t.Fatalf("text input: %q", got)
	}

	called := false
	p.confirm("ok?", func() { called = true })
	p.handleKey("n")
	if called || p.mode != modeList {
		t.Fatal("n cancels the confirmation")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("shellQuote: %s", got)
	}
}

func TestHelpScreen(t *testing.T) {
	p := testPanel(t)
	p.w, p.h = 60, 30
	p.handleKey("?")
	if p.mode != modeHelp {
		t.Fatal("? opens the help")
	}
	lines := strings.Join(p.viewHelp(), "\n")
	for _, h := range helpKeys {
		if !strings.Contains(lines, h.key) {
			t.Errorf("help lacks %q", h.key)
		}
	}
	p.handleKey("x")
	if p.mode != modeList {
		t.Fatal("any key closes the help")
	}
	if len(p.agents) != 0 {
		t.Fatal("the key that closes the help does nothing else")
	}
}

func TestSessionText(t *testing.T) {
	p := testPanel(t)
	p.titleOf["s1"] = "Fix the login"
	pr := p.d.Config.Profiles[0]
	cases := []struct {
		name, session, want string
	}{
		{"api", "", "new conversation"},  // automatic name, no title yet
		{"api 2", "s1", "Fix the login"}, // automatic name: the conversation title
		{"api x", "s1", "api x"},         // renamed: the name wins
		{"Pessoal", "", "Pessoal"},
	}
	for _, c := range cases {
		got := p.sessionText(agent{Name: c.name, Dir: "/p/api", Profile: pr.Name, Session: c.session})
		if got != c.want && !(c.want == "new conversation" && got == "nova conversa") {
			t.Errorf("sessionText(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// Claude does not record an account switch in the transcript, so Argus
// remembers it: the conversation reopens in the account it was switched to.
func TestRememberedProfileWins(t *testing.T) {
	p := testPanel(t)
	other := p.d.Config.Profiles[len(p.d.Config.Profiles)-1].Name
	p.prefs.Sessions["claude:abc"] = other
	p.prefs.Sessions["claude:gone"] = "profile-that-no-longer-exists"
	es := p.withRemembered([]history.Entry{
		{Tool: "claude", ID: "abc", Profile: p.d.Config.Profiles[0].Name},
		{Tool: "claude", ID: "gone", Profile: p.d.Config.Profiles[0].Name},
		{Tool: "opencode", ID: "abc", Profile: "x"},
	})
	if es[0].Profile != other {
		t.Errorf("switched conversation: %q, want %q", es[0].Profile, other)
	}
	if es[1].Profile != p.d.Config.Profiles[0].Name {
		t.Errorf("a deleted profile is ignored: %q", es[1].Profile)
	}
	if es[2].Profile != "x" {
		t.Errorf("same id in another tool is another conversation: %q", es[2].Profile)
	}
}

func TestPickAnyFolder(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"Code/api", "Code/Apps", "Code/.hidden", "Docs"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(home, "Code", "app.txt"), nil, 0o644)
	names := func(ps []project) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Rel)
		}
		return strings.Join(out, " ")
	}
	if got := names(browseDirs("~/Code/ap", home, 10)); got != "~/Code/api/ ~/Code/Apps/" {
		t.Fatalf("prefix: %q", got)
	}
	if got := names(browseDirs("~/Code/", home, 10)); got != "~/Code/ ~/Code/api/ ~/Code/Apps/" {
		t.Fatalf("inside a folder: %q", got)
	}
	if got := names(browseDirs("~/Code/.h", home, 10)); got != "~/Code/.hidden/" {
		t.Fatalf("hidden with a dot: %q", got)
	}
	p := testPanel(t)
	p.d.Paths.Home = home
	p.openPicker()
	for _, k := range []string{"~", "/", "C"} {
		p.keyPickDir(k)
	}
	p.keyPickDir("tab")
	if p.filter != "~/Code/" || len(p.filtered) == 0 || p.filtered[0].Path != filepath.Join(home, "Code") {
		t.Fatalf("tab goes into the folder: %q %+v", p.filter, p.filtered)
	}
}
