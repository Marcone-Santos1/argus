package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type benchRow struct {
	name, limit, value string
	ok                 *bool
}

func boolp(b bool) *bool { return &b }

func bench(args []string) error {
	secs := 60
	for i := 0; i < len(args); i++ {
		if (args[i] == "-s" || args[i] == "--seg") && i+1 < len(args) {
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				return fmt.Errorf("-s espera um número de segundos")
			}
			secs = n
			i++
		}
	}
	dur := time.Duration(secs) * time.Second
	if !exists(hookPath()) {
		return fmt.Errorf("rode `argus install` primeiro")
	}
	var rows []benchRow

	// 1. Hook: tempo por evento e processos criados.
	fmt.Println("medindo o hook (1.000 execuções)...")
	id := "benchhook"
	env := append(os.Environ(), "ARGUS_ID="+id)
	times := make([]time.Duration, 0, 1000)
	pid0 := probePID()
	for i := 0; i < 1000; i++ {
		arg := "prompt"
		if i%2 == 1 {
			arg = "stop"
		}
		cmd := exec.Command(hookPath(), arg)
		cmd.Env = env
		t := time.Now()
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook falhou: %w", err)
		}
		times = append(times, time.Since(t))
	}
	pid1 := probePID()
	_ = os.Remove(statePath(id))
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	p95 := times[len(times)*95/100]
	rows = append(rows, benchRow{"Tempo do hook por evento (p95)", "< 5 ms", ms(p95), boolp(p95 < 5*time.Millisecond)})
	perEvent := float64(pid1-pid0-1) / 1000
	rows = append(rows, benchRow{"Processos por evento do agente", "≤ 1", fmt.Sprintf("%.2f (inclui outros apps)", perEvent), boolp(perEvent < 1.2)})

	// 2. Painel.
	panelPID := findPanelPID()
	if panelPID == 0 {
		n := countPanelProcs()
		rows = append(rows, benchRow{"Processos com o painel fechado", "0", strconv.Itoa(n), boolp(n == 0)})
		rows = append(rows, benchRow{"Painel (CPU, memória, latência)", "", "painel fechado: abra `argus` e rode de novo", nil})
		printBench(rows)
		return nil
	}
	rows = append(rows, benchRow{"Processos com o painel fechado", "0", "painel aberto agora: feche (q) e rode de novo", nil})

	fmt.Printf("medindo o painel parado (%ds)...\n", secs)
	idle := cpuPercent(panelPID, dur)
	rows = append(rows, benchRow{"CPU do painel parado", "< 0,5%", pct(idle), boolp(idle >= 0 && idle < 0.5)})

	fmt.Printf("medindo o painel com 8 sessões simuladas mudando de estado (%ds)...\n", secs)
	stop := make(chan struct{})
	go func() {
		states := []string{stWorking, stWaiting, stDone}
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for n := 0; ; n++ {
			select {
			case <-stop:
				return
			case <-t.C:
				for i := 1; i <= 8; i++ {
					_ = writeStatus(fmt.Sprintf("bench%d", i), states[(n+i)%3])
				}
			}
		}
	}()
	busy := cpuPercent(panelPID, dur)
	close(stop)
	for i := 1; i <= 8; i++ {
		_ = os.Remove(statePath(fmt.Sprintf("bench%d", i)))
	}
	rows = append(rows, benchRow{"CPU do painel com carga (simulada)", "< 3%", pct(busy), boolp(busy >= 0 && busy < 3)})

	rss := rssMB(panelPID)
	rows = append(rows, benchRow{"Memória do painel (RSS)", "< 30 MB", fmt.Sprintf("%.1f MB", rss), boolp(rss > 0 && rss < 30)})

	fmt.Println("medindo evento -> lista (20 amostras)...")
	var lat []time.Duration
	for i := 0; i < 20; i++ {
		stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
		t := time.Now()
		_ = os.WriteFile(filepath.Join(stateDir, "_bench"), []byte(stamp), 0o644)
		for time.Since(t) < 2*time.Second {
			if b, _ := os.ReadFile(filepath.Join(stateDir, "_bench.ack")); string(b) == stamp {
				lat = append(lat, time.Since(t))
				break
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(filepath.Join(stateDir, "_bench"))
	_ = os.Remove(filepath.Join(stateDir, "_bench.ack"))
	if len(lat) == 0 {
		rows = append(rows, benchRow{"Do evento até a lista mudar", "< 100 ms", "sem resposta do painel", boolp(false)})
	} else {
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		worst := lat[len(lat)-1]
		rows = append(rows, benchRow{"Do evento até a lista mudar (pior de 20)", "< 100 ms", ms(worst), boolp(worst < 100*time.Millisecond && len(lat) == 20)})
	}

	m := map[string]float64{}
	if b, err := os.ReadFile(filepath.Join(stateDir, "_metrics")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if v, ok := m["startup_ms"]; ok {
		rows = append(rows, benchRow{"Abrir o painel", "< 150 ms", fmt.Sprintf("%.1f ms", v), boolp(v < 150)})
	}
	if v, ok := m["history_ms"]; ok {
		rows = append(rows, benchRow{"Abrir o histórico (última vez)", "< 150 ms", fmt.Sprintf("%.1f ms", v), boolp(v < 150)})
	} else {
		rows = append(rows, benchRow{"Abrir o histórico", "< 150 ms", "sem dados: aperte h uma vez", nil})
	}
	if v, ok := m["switch_ms"]; ok {
		rows = append(rows, benchRow{"Trocar de sessão (última troca)", "< 50 ms", fmt.Sprintf("%.1f ms", v), boolp(v < 50)})
	} else {
		rows = append(rows, benchRow{"Trocar de sessão", "< 50 ms", "sem dados: troque de sessão uma vez", nil})
	}
	printBench(rows)
	return nil
}

func printBench(rows []benchRow) {
	fmt.Println()
	fmt.Printf("%-44s %-10s %s\n", "medida", "limite", "resultado")
	fmt.Println(strings.Repeat("─", 90))
	for _, r := range rows {
		mark := "  "
		if r.ok != nil {
			if *r.ok {
				mark = "✓ "
			} else {
				mark = "✗ "
			}
		}
		fmt.Printf("%-44s %-10s %s%s\n", r.name, r.limit, mark, r.value)
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000) }
func pct(v float64) string {
	if v < 0 {
		return "erro ao medir"
	}
	return strings.Replace(fmt.Sprintf("%.2f%%", v), ".", ",", 1)
}

// probePID cria um processo e devolve seu PID (para contar processos criados).
func probePID() int {
	cmd := exec.Command("/bin/sh", "-c", "echo $$")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

func findPanelPID() int {
	if !sessionExists() {
		return 0
	}
	ps, err := listPanes()
	if err != nil {
		return 0
	}
	for _, p := range ps {
		if p.Role == "panel" && !p.Dead {
			return p.PID
		}
	}
	return 0
}

// Processos do Argus além deste bench (o servidor tmux se chama tmux, não conta).
func countPanelProcs() int {
	out, _ := exec.Command("pgrep", "-x", "argus").Output()
	n := 0
	for _, f := range strings.Fields(string(out)) {
		if f != strconv.Itoa(os.Getpid()) {
			n++
		}
	}
	return n
}

func cpuTime(pid int) (time.Duration, error) {
	out, err := exec.Command("ps", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, err
	}
	// [[dd-]hh:]mm:ss.ss
	s := strings.TrimSpace(string(out))
	days := 0
	if i := strings.Index(s, "-"); i >= 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	var total float64
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		total = total*60 + v
	}
	total += float64(days) * 86400
	return time.Duration(total * float64(time.Second)), nil
}

func cpuPercent(pid int, dur time.Duration) float64 {
	a, err := cpuTime(pid)
	if err != nil {
		return -1
	}
	time.Sleep(dur)
	b, err := cpuTime(pid)
	if err != nil {
		return -1
	}
	return float64(b-a) / float64(dur) * 100
}

func rssMB(pid int) float64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return kb / 1024
}
