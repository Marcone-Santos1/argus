package status

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abc123")
	if s, since := Read(path); s != Idle || !since.IsZero() {
		t.Fatalf("no file: %q %v", s, since)
	}
	if err := Write(path, Working); err != nil {
		t.Fatal(err)
	}
	if s, since := Read(path); s != Working || since.IsZero() {
		t.Fatalf("after writing: %q %v", s, since)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := Read(path); s != Idle {
		t.Fatalf("empty file (write in progress): %q", s)
	}
}

func TestReadOpencode(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ses_1", "working\n/p/api\n")
	write("ses_2", "done\n")         // no folder: ignored
	write(".tmp", "working\n/p/x\n") // hidden: ignored
	got := ReadOpencode(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 state, got %v", got)
	}
	oc := got["ses_1"]
	if oc.Status != Working || oc.Dir != "/p/api" || oc.Session != "ses_1" {
		t.Fatalf("state: %+v", oc)
	}
	if len(ReadOpencode(filepath.Join(dir, "missing"))) != 0 {
		t.Fatal("a missing folder gives an empty map")
	}
}

func TestShort(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Second:     "0s",
		0:                "0s",
		59 * time.Second: "59s",
		90 * time.Second: "1m",
		59 * time.Minute: "59m",
		3 * time.Hour:    "3h",
		49 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := Short(d); got != want {
			t.Errorf("Short(%v) = %s, want %s", d, got, want)
		}
	}
}
