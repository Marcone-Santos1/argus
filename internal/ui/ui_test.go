package ui

import (
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
		{"\x1b[1;5C", nil}, // seta com modificador: ignorada
	}
	for _, c := range cases {
		if got := parseKeys([]byte(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseKeys(%q) = %q, quer %q", c.in, got, c.want)
		}
	}
}

func TestFit(t *testing.T) {
	if got := fit("abc", 5); got != "abc  " {
		t.Errorf("completa: %q", got)
	}
	if got := fit("abcdef", 4); got != "abc…" {
		t.Errorf("corta: %q", got)
	}
	if got := fit("ação", 4); got != "ação" {
		t.Errorf("conta runas, não bytes: %q", got)
	}
	if fit("x", 0) != "" {
		t.Error("largura 0")
	}
}

func TestLayoutKeepsFocusVisible(t *testing.T) {
	var body []string
	for i := 0; i < 50; i++ {
		body = append(body, string(rune('A'+i%26)))
	}
	out := layout(body, []string{"rodapé"}, 11, 40)
	if len(out) != 11 || out[10] != "rodapé" {
		t.Fatalf("altura e rodapé: %q", out)
	}
	if !strings.Contains(strings.Join(out[:10], ""), body[40]) {
		t.Fatal("a linha em foco precisa aparecer")
	}
	short := layout([]string{"a"}, []string{"f"}, 4, 0)
	if !reflect.DeepEqual(short, []string{"a", "", "", "f"}) {
		t.Fatalf("completa com linhas vazias: %q", short)
	}
}

func TestWrapHints(t *testing.T) {
	lines := wrapHints([]string{"aaa", "bbb", "ccc"}, 10)
	if len(lines) != 2 {
		t.Fatalf("quebra em duas linhas: %q", lines)
	}
}

func testPanel(t *testing.T) *Panel {
	t.Helper()
	home := t.TempDir()
	cfg, err := config.Parse(config.DefaultTOML, home)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Paths: paths.For(home, "x"), Config: cfg, Tmux: tmux.New("argus-ui-test-inexistente")})
}

func TestRebuildItemsGroupsByFolder(t *testing.T) {
	p := testPanel(t)
	now := time.Now()
	p.agents = []agent{
		{ID: "a1", Dir: "/p/api", Profile: "claude-pessoal", Name: "api"},
		{ID: "a2", Dir: "/p/site", Profile: "opencode", Name: "site", Session: "ses_aberta"},
		{ID: "a3", Dir: "/p/api", Profile: "claude-work", Name: "api 2"},
	}
	p.histAll = []history.Entry{
		{Tool: "claude", ID: "c1", Dir: "/p/docs", Title: "Docs", Updated: now},
		{Tool: "opencode", ID: "ses_aberta", Dir: "/p/site", Title: "Já aberta", Updated: now},
		{Tool: "claude", ID: "c2", Dir: "/p/api", Title: "Api antiga", Updated: now.Add(-time.Hour)},
		{Tool: "claude", ID: "c3", Dir: "/p/api", Title: "Api 3", Updated: now.Add(-2 * time.Hour)},
		{Tool: "claude", ID: "c4", Dir: "/p/api", Title: "Api 4", Updated: now.Add(-3 * time.Hour)},
		{Tool: "claude", ID: "c5", Dir: "/p/api", Title: "Api 5 (passa do limite)", Updated: now.Add(-4 * time.Hour)},
	}
	p.rebuildItems("")
	var got []string
	for _, it := range p.items {
		if it.agent >= 0 {
			got = append(got, it.dir+" sessão "+p.agents[it.agent].ID)
		} else {
			got = append(got, it.dir+" conversa "+it.entry.ID)
		}
	}
	want := []string{
		"/p/api sessão a1", "/p/api sessão a3",
		"/p/api conversa c2", "/p/api conversa c3", "/p/api conversa c4",
		"/p/site sessão a2", // a conversa já aberta não se repete
		"/p/docs conversa c1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lista:\n%s\nquer:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if i := p.nthAgentItem(2); i != 5 {
		t.Errorf("tecla 3 vai para a terceira sessão (linha 5): %d", i)
	}

	// A seleção acompanha o item, não a posição.
	p.sel = 5
	key := p.selKey()
	p.agents = p.agents[1:] // a1 fechou
	p.rebuildItems(key)
	if p.selKey() != key {
		t.Fatalf("seleção perdida: %s -> %s", key, p.selKey())
	}

	// Filtro.
	p.listFilter = "docs"
	p.rebuildItems("")
	if len(p.items) != 1 || p.items[0].entry.ID != "c1" {
		t.Fatalf("filtro: %+v", p.items)
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
		t.Fatalf("sessão nova pega a conversa mais recente da pasta, criada depois dela: %+v", oc)
	}
	oc, ok = p.opencodeFor(&p.agents[1])
	if !ok || oc.Status != status.Waiting {
		t.Fatalf("sessão retomada usa o id: %+v", oc)
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
	if p.listFilter != "api" || len(p.items) != 1 {
		t.Fatalf("filtro digitado: %q %d", p.listFilter, len(p.items))
	}
	p.handleKey("enter")
	if p.mode != modeList || p.listFilter != "api" {
		t.Fatal("Enter sai do filtro e mantém")
	}
	p.handleKey("esc")
	if p.listFilter != "" || len(p.items) != 2 {
		t.Fatal("Esc na lista limpa o filtro")
	}

	got := ""
	p.ask("nome:", "abc", func(s string) { got = s })
	p.handleKey("backspace")
	p.handleKey("Z")
	p.handleKey("enter")
	if got != "abZ" || p.mode != modeList {
		t.Fatalf("entrada de texto: %q", got)
	}

	called := false
	p.confirm("ok?", func() { called = true })
	p.handleKey("n")
	if called || p.mode != modeList {
		t.Fatal("n cancela a confirmação")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("shellQuote: %s", got)
	}
}
