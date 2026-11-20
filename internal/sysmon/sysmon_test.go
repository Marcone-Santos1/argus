package sysmon

import (
	"reflect"
	"testing"
	"time"
)

// fake simulates probes every 2 s with a fixed process rate.
type fake struct {
	now time.Time
	pid int
}

func newFake(m *Monitor, perStep int) *fake {
	f := &fake{now: time.Unix(1_000_000, 0), pid: 1000}
	m.Now = func() time.Time { return f.now }
	m.probes = true
	m.Probe = func() int {
		f.pid += perStep + 1 // +1 = the probe itself
		return f.pid
	}
	return f
}

func TestRateAndStorm(t *testing.T) {
	m := &Monitor{}
	f := newFake(m, 10) // 10 processes every 2 s = 300/min
	m.Sample()
	var r Reading
	for i := 0; i < 5; i++ {
		f.now = f.now.Add(2 * time.Second)
		r = m.Sample()
	}
	if r.PerMinute < 299 || r.PerMinute > 301 {
		t.Fatalf("rate: %.1f/min, want 300", r.PerMinute)
	}
	if r.Storm {
		t.Fatal("300/min is not a storm")
	}

	m2 := &Monitor{}
	f2 := newFake(m2, 100) // 3000/min
	m2.Sample()
	for i := 0; i < 14; i++ { // 28 s
		f2.now = f2.now.Add(2 * time.Second)
		if r = m2.Sample(); r.Storm {
			t.Fatalf("storm before %v", StormFor)
		}
	}
	for i := 0; i < 2; i++ {
		f2.now = f2.now.Add(2 * time.Second)
		r = m2.Sample()
	}
	if !r.Storm || r.PerMinute < StormRate {
		t.Fatalf("after 30 s above the limit: %+v", r)
	}
}

func TestCPUReading(t *testing.T) {
	m := New()
	m.Sample()
	var r Reading
	for i := 0; i < 20 && r.CPU <= 0; i++ { // the counters move in steps
		time.Sleep(100 * time.Millisecond)
		for j := 0; j < 1e6; j++ { // burn a little CPU
			_ = j * j
		}
		r = m.Sample()
	}
	if r.CPU < 0 || r.CPU > 100 {
		t.Fatalf("CPU outside 0-100: %.2f", r.CPU)
	}
}

func TestTopFromPS(t *testing.T) {
	out := `   10 /sbin/launchd
 5001 /opt/x/node
 5002 node
 5003 /bin/zsh
 5004 /usr/local/bin/node
 junk
`
	got := topFromPS(out, 5000, 2)
	if !reflect.DeepEqual(got, []string{"node", "zsh"}) {
		t.Fatalf("top: %v", got)
	}
}
