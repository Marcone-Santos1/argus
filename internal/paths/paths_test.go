package paths

import "testing"

func TestForSeparatesTestServers(t *testing.T) {
	real := For("/h", "argus")
	if real.State != "/h/.local/state/argus" {
		t.Fatalf("real server state: %s", real.State)
	}
	if def := For("/h", ""); def.State != real.State {
		t.Fatalf("an empty socket means the real server: %s", def.State)
	}
	test := For("/h", "argustest")
	if test.State == real.State {
		t.Fatal("a test server must not share state with the real one")
	}
	if test.Share != real.Share || test.Conf != real.Conf {
		t.Fatal("hook and config are the same for every server")
	}
}

func TestExpandAndPretty(t *testing.T) {
	p := For("/Users/ana", "")
	cases := map[string]string{
		"~":            "/Users/ana",
		"~/Projects":   "/Users/ana/Projects",
		"/opt/x":       "/opt/x",
		"relative/~/a": "relative/~/a",
	}
	for in, want := range cases {
		if got := p.Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
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
			t.Errorf("Pretty(%q) = %q, want %q", in, got, want)
		}
	}
}
