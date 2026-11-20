// Package sysmon mede o uso de CPU da máquina e quantos processos estão sendo
// criados por minuto, para o rodapé do painel e o alarme de tempestade de
// processos (o que travou a máquina com outra ferramenta do tipo).
package sysmon

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StormRate é a taxa (processos/min) que, sustentada por StormFor, dispara o alarme.
const (
	StormRate = 2000
	StormFor  = 30 * time.Second
)

// Reading é uma leitura do monitor.
type Reading struct {
	CPU        float64 // % de toda a máquina, 0-100; <0 = sem dado
	PerMinute  float64 // processos criados por minuto; <0 = sem dado
	Storm      bool    // taxa acima de StormRate por StormFor
	StormSince time.Time
}

type sample struct {
	at  time.Time
	pid int
}

// Monitor acumula leituras. Cada Sample cria um processo /usr/bin/true para
// descobrir o próximo PID (o macOS não expõe esse contador); o próprio teste
// é descontado da taxa.
type Monitor struct {
	busy, total uint64
	samples     []sample
	stormStart  time.Time
	Probe       func() int // devolve um PID novo; trocável em testes
	Now         func() time.Time
}

// New cria um monitor com a sonda padrão.
func New() *Monitor {
	return &Monitor{Probe: probePID, Now: time.Now}
}

func probePID() int {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Start(); err != nil {
		return 0
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

// Sample faz uma leitura. A primeira chamada só prepara a base.
func (m *Monitor) Sample() Reading {
	r := Reading{CPU: -1, PerMinute: -1}
	if b, t, ok := cpuTicks(); ok {
		if m.total != 0 && t > m.total {
			r.CPU = float64(b-m.busy) / float64(t-m.total) * 100
		}
		m.busy, m.total = b, t
	}
	now := m.Now()
	pid := m.Probe()
	if pid > 0 {
		m.samples = append(m.samples, sample{now, pid})
		// Janela de 60 s.
		for len(m.samples) > 2 && now.Sub(m.samples[0].at) > time.Minute {
			m.samples = m.samples[1:]
		}
		first := m.samples[0]
		if span := now.Sub(first.at); span >= time.Second && pid > first.pid {
			created := float64(pid-first.pid) - float64(len(m.samples)-1) // desconta as sondas
			r.PerMinute = max(0, created) / span.Minutes()
		} else if pid < first.pid {
			m.samples = m.samples[len(m.samples)-1:] // PIDs deram a volta
		}
	}
	if r.PerMinute >= StormRate {
		if m.stormStart.IsZero() {
			m.stormStart = now
		}
		if now.Sub(m.stormStart) >= StormFor {
			r.Storm = true
			r.StormSince = m.stormStart
		}
	} else {
		m.stormStart = time.Time{}
	}
	return r
}

// TopCommands lista os programas com mais processos vivos acima de sincePID
// (os criados recentemente), mais frequentes primeiro. Roda `ps` uma vez.
func TopCommands(sincePID, n int) []string {
	out, err := exec.Command("/bin/ps", "-A", "-o", "pid=,comm=").Output()
	if err != nil {
		return nil
	}
	return topFromPS(string(out), sincePID, n)
}

func topFromPS(out string, sincePID, n int) []string {
	count := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= sincePID {
			continue
		}
		count[filepath.Base(strings.Join(f[1:], " "))]++
	}
	names := make([]string, 0, len(count))
	for k := range count {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if count[names[i]] != count[names[j]] {
			return count[names[i]] > count[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > n {
		names = names[:n]
	}
	return names
}

// FirstPID é o PID mais antigo da janela atual (base para TopCommands).
func (m *Monitor) FirstPID() int {
	if len(m.samples) == 0 {
		return 0
	}
	return m.samples[0].pid
}
