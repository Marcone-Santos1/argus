package sysmon

import (
	"reflect"
	"testing"
	"time"
)

// fakeClock simula sondas a cada 2 s com uma taxa fixa de processos.
type fake struct {
	now time.Time
	pid int
}

func newFake(m *Monitor, perStep int) *fake {
	f := &fake{now: time.Unix(1_000_000, 0), pid: 1000}
	m.Now = func() time.Time { return f.now }
	m.Probe = func() int {
		f.pid += perStep + 1 // +1 = a própria sonda
		return f.pid
	}
	return f
}

func TestRateAndStorm(t *testing.T) {
	m := &Monitor{}
	f := newFake(m, 10) // 10 processos a cada 2 s = 300/min
	m.Sample()
	var r Reading
	for i := 0; i < 5; i++ {
		f.now = f.now.Add(2 * time.Second)
		r = m.Sample()
	}
	if r.PerMinute < 299 || r.PerMinute > 301 {
		t.Fatalf("taxa: %.1f/min, quer 300", r.PerMinute)
	}
	if r.Storm {
		t.Fatal("300/min não é tempestade")
	}

	m2 := &Monitor{}
	f2 := newFake(m2, 100) // 3000/min
	m2.Sample()
	for i := 0; i < 14; i++ { // 28 s
		f2.now = f2.now.Add(2 * time.Second)
		if r = m2.Sample(); r.Storm {
			t.Fatalf("tempestade antes de %v", StormFor)
		}
	}
	for i := 0; i < 2; i++ {
		f2.now = f2.now.Add(2 * time.Second)
		r = m2.Sample()
	}
	if !r.Storm || r.PerMinute < StormRate {
		t.Fatalf("depois de 30 s acima do limite: %+v", r)
	}
}

func TestCPUReading(t *testing.T) {
	m := New()
	m.Sample()
	var r Reading
	for i := 0; i < 20 && r.CPU <= 0; i++ { // os contadores andam em passos
		time.Sleep(100 * time.Millisecond)
		for j := 0; j < 1e6; j++ { // gasta um pouco de CPU
			_ = j * j
		}
		r = m.Sample()
	}
	if r.CPU < 0 || r.CPU > 100 {
		t.Fatalf("CPU fora de 0-100: %.2f", r.CPU)
	}
}

func TestTopFromPS(t *testing.T) {
	out := `   10 /sbin/launchd
 5001 /opt/x/node
 5002 node
 5003 /bin/zsh
 5004 /usr/local/bin/node
 lixo
`
	got := topFromPS(out, 5000, 2)
	if !reflect.DeepEqual(got, []string{"node", "zsh"}) {
		t.Fatalf("top: %v", got)
	}
}
