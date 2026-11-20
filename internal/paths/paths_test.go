package paths

import "testing"

func TestForSeparatesTestServers(t *testing.T) {
	real := For("/h", "argus")
	if real.State != "/h/.local/state/argus" {
		t.Fatalf("estado do servidor real: %s", real.State)
	}
	if def := For("/h", ""); def.State != real.State {
		t.Fatalf("socket vazio deveria ser o real: %s", def.State)
	}
	test := For("/h", "argustest")
	if test.State == real.State {
		t.Fatal("servidor de teste não pode dividir o estado com o real")
	}
	if test.Share != real.Share || test.Conf != real.Conf {
		t.Fatal("hook e config são os mesmos para qualquer servidor")
	}
}

func TestExpandAndPretty(t *testing.T) {
	p := For("/Users/ana", "")
	cases := map[string]string{
		"~":            "/Users/ana",
		"~/Projects":   "/Users/ana/Projects",
		"/opt/x":       "/opt/x",
		"relativo/~/a": "relativo/~/a",
	}
	for in, want := range cases {
		if got := p.Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, quer %q", in, got, want)
		}
	}
	pretty := map[string]string{
		"/Users/ana":            "~",
		"/Users/ana/Projects/x": "~/Projects/x",
		"/Users":                "/Users",
		"/tmp/y":                "/tmp/y",
	}
	for in, want := range pretty {
		if got := p.Pretty(in); got != want {
			t.Errorf("Pretty(%q) = %q, quer %q", in, got, want)
		}
	}
}
