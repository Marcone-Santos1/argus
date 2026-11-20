// Package bench mede o Argus contra as metas de desempenho da especificação.
package bench

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Marcone-Santos1/argus/internal/paths"
	"github.com/Marcone-Santos1/argus/internal/status"
	"github.com/Marcone-Santos1/argus/internal/tmux"
)

// Row é uma linha do relatório. OK nil = só informativa.
type Row struct {
	Name, Limit, Value string
	OK                 *bool
}

func ok(b bool) *bool { return &b }

// Run executa as medições; secs é a duração de cada medição de CPU.
func Run(p paths.Paths, t *tmux.Client, secs int, out io.Writer) error {
	dur := time.Duration(secs) * time.Second
	if _, err := os.Stat(p.Hook()); err != nil {
		return fmt.Errorf("rode `argus install` primeiro")
	}
	var rows []Row

	fmt.Fprintln(out, "medindo o hook (1.000 execuções)...")
	p95, perEvent, err := hookCost(p, 1000)
	if err != nil {
		return err
	}
	rows = append(rows,
		Row{"Tempo do hook por evento (p95)", "< 5 ms", ms(p95), ok(p95 < 5*time.Millisecond)},
		Row{"Processos por evento do agente", "≤ 1", fmt.Sprintf("%.2f (inclui outros apps)", perEvent), ok(perEvent < 1.2)})

	pid := panelPID(t)
	if pid == 0 {
		n := countArgus()
		rows = append(rows,
			Row{"Processos com o painel fechado", "0", strconv.Itoa(n), ok(n == 0)},
			Row{"Painel (CPU, memória, latência)", "", "painel fechado: abra `argus` e rode de novo", nil})
		Print(out, rows)
		return nil
	}
	rows = append(rows, Row{"Processos com o painel fechado", "0", "painel aberto agora: feche (q) e rode de novo", nil})

	fmt.Fprintf(out, "medindo o painel parado (%ds)...\n", secs)
	idle := cpuPercent(pid, dur)
	rows = append(rows, Row{"CPU do painel parado (com o monitor do rodapé)", "< 0,5%", pct(idle), ok(idle >= 0 && idle < 0.5)})

	fmt.Fprintf(out, "medindo o painel com 8 sessões simuladas mudando de estado (%ds)...\n", secs)
	stop := make(chan struct{})
	go simulate(p, stop)
	busy := cpuPercent(pid, dur)
	close(stop)
	for i := 1; i <= 8; i++ {
		_ = os.Remove(p.Status(fmt.Sprintf("bench%d", i)))
	}
	rows = append(rows, Row{"CPU do painel com carga (simulada)", "< 3%", pct(busy), ok(busy >= 0 && busy < 3)})

	rss := rssMB(pid)
	rows = append(rows, Row{"Memória do painel (RSS)", "< 30 MB", fmt.Sprintf("%.1f MB", rss), ok(rss > 0 && rss < 30)})

	fmt.Fprintln(out, "medindo evento -> lista (20 amostras)...")
	lat := eventLatency(p, 20)
	if len(lat) == 0 {
		rows = append(rows, Row{"Do evento até a lista mudar", "< 100 ms", "sem resposta do painel", ok(false)})
	} else {
		worst := lat[len(lat)-1]
		rows = append(rows, Row{"Do evento até a lista mudar (pior de 20)", "< 100 ms", ms(worst), ok(worst < 100*time.Millisecond && len(lat) == 20)})
	}

	m := map[string]float64{}
	if b, err := os.ReadFile(p.Metrics()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	metric := func(key, name, limit string, max float64, hint string) {
		if v, found := m[key]; found {
			rows = append(rows, Row{name, limit, fmt.Sprintf("%.1f ms", v), ok(v < max)})
		} else {
			rows = append(rows, Row{name, limit, hint, nil})
		}
	}
	metric("startup_ms", "Abrir o painel", "< 150 ms", 150, "sem dados")
	metric("switch_ms", "Trocar de sessão (última troca)", "< 50 ms", 50, "sem dados: troque de sessão uma vez")
	metric("history_ms", "Abrir o histórico (última vez)", "< 150 ms", 150, "sem dados: aperte h uma vez")
	Print(out, rows)
	return nil
}

// Print escreve o relatório.
func Print(out io.Writer, rows []Row) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%-48s %-10s %s\n", "medida", "limite", "resultado")
	fmt.Fprintln(out, strings.Repeat("─", 94))
	for _, r := range rows {
		mark := "  "
		if r.OK != nil {
			mark = map[bool]string{true: "✓ ", false: "✗ "}[*r.OK]
		}
		fmt.Fprintf(out, "%-48s %-10s %s%s\n", r.Name, r.Limit, mark, r.Value)
	}
}

// hookCost roda o hook n vezes e devolve o p95 e quantos processos cada evento criou.
func hookCost(p paths.Paths, n int) (time.Duration, float64, error) {
	env := append(os.Environ(), "ARGUS_ID=benchhook")
	times := make([]time.Duration, 0, n)
	pid0 := probePID()
	for i := 0; i < n; i++ {
		arg := "prompt"
		if i%2 == 1 {
			arg = "stop"
		}
		cmd := exec.Command(p.Hook(), arg)
		cmd.Env = env
		t := time.Now()
		if err := cmd.Run(); err != nil {
			return 0, 0, fmt.Errorf("hook falhou: %w", err)
		}
		times = append(times, time.Since(t))
	}
	pid1 := probePID()
	_ = os.Remove(p.Status("benchhook"))
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times[len(times)*95/100], float64(pid1-pid0-1) / float64(n), nil
}

func simulate(p paths.Paths, stop <-chan struct{}) {
	states := []string{status.Working, status.Waiting, status.Done}
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for n := 0; ; n++ {
		select {
		case <-stop:
			return
		case <-t.C:
			for i := 1; i <= 8; i++ {
				_ = status.Write(p.Status(fmt.Sprintf("bench%d", i)), states[(n+i)%3])
			}
		}
	}
}

func eventLatency(p paths.Paths, n int) []time.Duration {
	var lat []time.Duration
	bench := filepath.Join(p.State, "_bench")
	ack := bench + ".ack"
	for i := 0; i < n; i++ {
		stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
		t := time.Now()
		_ = os.WriteFile(bench, []byte(stamp), 0o644)
		for time.Since(t) < 2*time.Second {
			if b, _ := os.ReadFile(ack); string(b) == stamp {
				lat = append(lat, time.Since(t))
				break
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(bench)
	_ = os.Remove(ack)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return lat
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000) }

func pct(v float64) string {
	if v < 0 {
		return "erro ao medir"
	}
	return strings.Replace(fmt.Sprintf("%.2f%%", v), ".", ",", 1)
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

func panelPID(t *tmux.Client) int {
	if !t.HasSession() {
		return 0
	}
	ps, err := t.ListPanes()
	if err != nil {
		return 0
	}
	for _, p := range ps {
		if p.Role == tmux.RolePanel && !p.Dead {
			return p.PID
		}
	}
	return 0
}

// countArgus conta processos argus além deste (o servidor tmux se chama tmux).
func countArgus() int {
	out, _ := exec.Command("pgrep", "-x", "argus").Output()
	n := 0
	for _, f := range strings.Fields(string(out)) {
		if f != strconv.Itoa(os.Getpid()) {
			n++
		}
	}
	return n
}

// ParseCPUTime interpreta o tempo de CPU do ps: [[dd-]hh:]mm:ss.ss.
func ParseCPUTime(s string) time.Duration {
	s = strings.TrimSpace(s)
	days := 0
	if i := strings.Index(s, "-"); i >= 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	var total float64
	for _, part := range strings.Split(s, ":") {
		v, _ := strconv.ParseFloat(part, 64)
		total = total*60 + v
	}
	total += float64(days) * 86400
	return time.Duration(total * float64(time.Second))
}

func cpuTime(pid int) (time.Duration, error) {
	out, err := exec.Command("ps", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, err
	}
	return ParseCPUTime(string(out)), nil
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
