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
		t.Fatalf("sem arquivo: %q %v", s, since)
	}
	if err := Write(path, Working); err != nil {
		t.Fatal(err)
	}
	if s, since := Read(path); s != Working || since.IsZero() {
		t.Fatalf("depois de gravar: %q %v", s, since)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := Read(path); s != Idle {
		t.Fatalf("arquivo vazio (escrita no meio): %q", s)
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
	write("ses_2", "done\n")         // sem pasta: ignorado
	write(".tmp", "working\n/p/x\n") // oculto: ignorado
	got := ReadOpencode(dir)
	if len(got) != 1 {
		t.Fatalf("esperava 1 estado, veio %v", got)
	}
	oc := got["ses_1"]
	if oc.Status != Working || oc.Dir != "/p/api" || oc.Session != "ses_1" {
		t.Fatalf("estado: %+v", oc)
	}
	if len(ReadOpencode(filepath.Join(dir, "nao-existe"))) != 0 {
		t.Fatal("pasta ausente deveria dar vazio")
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
			t.Errorf("Short(%v) = %s, quer %s", d, got, want)
		}
	}
}
