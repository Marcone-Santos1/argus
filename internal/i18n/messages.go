package i18n

// messages maps a key to its text in each language: {EN, PT}.
var messages = map[string][2]string{
	// List.
	"list.empty1":      {" No open sessions. n starts one;", " Nenhuma sessão aberta. n cria uma;"},
	"list.empty2":      {" Enter on a conversation resumes it.", " Enter numa conversa a retoma."},
	"list.nothing":     {" nothing found", " nada encontrado"},
	"list.sessions":    {"%d sessions", "%d sessões"},
	"list.session":     {"1 session", "1 sessão"},
	"list.stats":       {"CPU %.0f%%  proc/min %.0f  ", "CPU %.0f%%  proc/min %.0f  "},
	"list.storm":       {" ⚠ %.0f processes/min", " ⚠ %.0f processos/min"},
	"hint.enter":       {"⏎ enter", "⏎ entrar"},
	"hint.toggle":      {"^\\ switch", "^\\ alterna"},
	"hint.tab":         {"tab attention", "tab atenção"},
	"hint.new":         {"n new", "n nova"},
	"hint.history":     {"h history", "h histórico"},
	"hint.worktree":    {"w worktree", "w worktree"},
	"hint.filter":      {"/ filter", "/ filtra"},
	"hint.rename":      {"r rename", "r nome"},
	"hint.hide":        {"esc hide", "esc esconde"},
	"hint.close":       {"x close", "x fecha"},
	"hint.zoom":        {"z zoom", "z cheia"},
	"hint.quit":        {"q quit", "q sai"},
	"hint.type":        {"type to filter", "digite para filtrar"},
	"hint.pick":        {"↑↓ choose", "↑↓ escolher"},
	"hint.ok":          {"⏎ ok", "⏎ ok"},
	"hint.cancel":      {"esc cancel", "esc cancela"},
	"hint.used":        {"· used before", "· usado antes"},
	"hint.open":        {"⏎ open", "⏎ abrir"},
	"hint.back":        {"esc back", "esc voltar"},
	"hint.typeHistory": {"type to filter (project, title, account)", "digite para filtrar (projeto, título, conta)"},
	"hint.resume":      {"⏎ resume", "⏎ retomar"},

	// Pickers.
	"pick.project":         {"New session: choose the project", "Nova sessão: escolha o projeto"},
	"pick.profile":         {"New session: choose the profile", "Nova sessão: escolha o perfil"},
	"pick.resumeAccount":   {"Resume conversation: choose the account", "Retomar conversa: escolha a conta"},
	"pick.worktreeProfile": {"Worktree session: choose the profile", "Sessão na worktree: escolha o perfil"},
	"history.title":        {"History: %d conversations", "Histórico: %d conversas"},
	"history.untitled":     {"(untitled)", "(sem título)"},

	// Actions and messages.
	"msg.closed":          {"closed: %s", "encerrada: %s"},
	"msg.active":          {"%s is active. Close it? (y/n)", "%s está ativa. Encerrar? (s/n)"},
	"msg.deleteWorktree":  {"Delete the worktree %s? (y/n)", "Apagar a worktree %s? (s/n)"},
	"msg.worktreeDeleted": {"worktree deleted; the branch stays in the repository", "worktree apagada; o branch continua no repositório"},
	"msg.noAttention":     {"no session is waiting for you", "nenhuma sessão esperando você"},
	"msg.pickRepo":        {"select a folder with a git repository", "selecione uma pasta com repositório git"},
	"msg.branchPrompt":    {"Worktree branch in %s:", "Branch da worktree em %s:"},
	"msg.renamePrompt":    {"New session name:", "Novo nome da sessão:"},
	"msg.limit":           {"There are already %d sessions (limit %d). Open one more? (y/n)", "Já são %d sessões (limite %d). Abrir mais uma? (s/n)"},
	"msg.badID":           {"invalid conversation id", "id de conversa inválido"},
	"msg.noProfile":       {"no profile configured for %s", "nenhum perfil configurado para %s"},
	"msg.stormFlash":      {"Process storm: %.0f/min.%s", "Tempestade de processos: %.0f/min.%s"},
	"msg.stormTop":        {" Most common: %s.", " Mais comum: %s."},
	"notify.stormTitle":   {"Argus: process storm", "Argus: tempestade de processos"},
	"notify.stormBody":    {"%.0f processes per minute.%s", "%.0f processos por minuto.%s"},
	"notify.waiting":      {"Waiting for you.", "Esperando você."},
	"notify.done":         {"Finished.", "Terminou."},

	// Worktrees.
	"wt.notRepo":   {"the folder is not a git repository", "a pasta não é um repositório git"},
	"wt.badBranch": {"invalid branch name: %q", "nome de branch inválido: %q"},
	"wt.dirty":     {"the worktree has uncommitted changes; it was kept", "a worktree tem mudanças sem commit; ela foi mantida"},

	// tmux.
	"tmux.placeholder": {"  No session open here. Pick one in the list or press n.", "  Nenhuma sessão aberta aqui. Escolha uma na lista ou aperte n."},

	// Command line.
	"cli.usage": {`argus: a panel for many Claude Code, opencode and Gemini sessions

usage:
  argus              open the panel (starts the tmux server if needed)
  argus install      install hooks, the opencode plugin, tmux config and a default config
  argus uninstall    remove everything install added
  argus bench [-s N] measure performance (N seconds per CPU measurement, default 60)
`, `argus: painel para várias sessões de Claude Code, opencode e Gemini

uso:
  argus              abre o painel (cria o servidor tmux se preciso)
  argus install      instala hooks, plugin do opencode, config do tmux e config padrão
  argus uninstall    remove tudo o que o install colocou
  argus bench [-s N] mede o desempenho (N segundos por medição de CPU, padrão 60)
`},
	"cli.needInstall":  {"run `argus install` first", "rode `argus install` primeiro"},
	"cli.insideArgus":  {"you are already inside Argus (Ctrl+\\ goes back to the list)", "você já está dentro do Argus (Ctrl+\\ volta para a lista)"},
	"cli.noPanel":      {"could not find the panel in the tmux session; run `tmux -L argus kill-server` and open again", "não achei o painel na sessão do tmux; rode `tmux -L argus kill-server` e abra de novo"},
	"cli.panelOutside": {"the panel runs inside the Argus tmux; use `argus`", "o painel roda dentro do tmux do Argus; use `argus`"},
	"cli.tmuxReloaded": {"tmux:             config reloaded in the running server", "tmux:             config recarregada no servidor em uso"},
	"cli.serverKept":   {"the Argus server keeps its sessions; close it with `tmux -L argus kill-server`", "o servidor do Argus continua com suas sessões; feche com `tmux -L argus kill-server`"},
	"cli.badConfig":    {"invalid config.toml", "config.toml inválido"},
	"install.new":      {"(new)", "(novo)"},
	"install.kept":     {"(kept)", "(mantido)"},
	"install.already":  {"(already installed)", "(já instalados)"},
	"install.added":    {"(+%d events; backup next to it)", "(+%d eventos; backup ao lado)"},
	"install.badJSON":  {"%s: not valid JSON; nothing was changed", "%s: não é JSON válido; nada foi alterado"},
	"install.removed":  {"removed:", "removido:"},

	// Bench.
	"bench.hook":          {"measuring the hook (1,000 runs)...", "medindo o hook (1.000 execuções)..."},
	"bench.idle":          {"measuring the idle panel (%ds)...", "medindo o painel parado (%ds)..."},
	"bench.load":          {"measuring the panel with 8 simulated sessions changing state (%ds)...", "medindo o painel com 8 sessões simuladas mudando de estado (%ds)..."},
	"bench.latency":       {"measuring event -> list (20 samples)...", "medindo evento -> lista (20 amostras)..."},
	"bench.header":        {"measure|limit|result", "medida|limite|resultado"},
	"bench.hookTime":      {"Hook time per event (p95)", "Tempo do hook por evento (p95)"},
	"bench.perEvent":      {"Processes per agent event", "Processos por evento do agente"},
	"bench.perEventValue": {"%.2f (includes other apps)", "%.2f (inclui outros apps)"},
	"bench.closed":        {"Processes with the panel closed", "Processos com o painel fechado"},
	"bench.panelClosed":   {"Panel (CPU, memory, latency)", "Painel (CPU, memória, latência)"},
	"bench.openPanel":     {"panel closed: open `argus` and run again", "painel fechado: abra `argus` e rode de novo"},
	"bench.closeFirst":    {"panel open now: close it (q) and run again", "painel aberto agora: feche (q) e rode de novo"},
	"bench.cpuIdle":       {"Panel CPU, idle (with the footer monitor)", "CPU do painel parado (com o monitor do rodapé)"},
	"bench.cpuLoad":       {"Panel CPU under load (simulated)", "CPU do painel com carga (simulada)"},
	"bench.rss":           {"Panel memory (RSS)", "Memória do painel (RSS)"},
	"bench.event":         {"Event to list update (worst of 20)", "Do evento até a lista mudar (pior de 20)"},
	"bench.noAnswer":      {"no answer from the panel", "sem resposta do painel"},
	"bench.startup":       {"Open the panel", "Abrir o painel"},
	"bench.switch":        {"Switch session (last switch)", "Trocar de sessão (última troca)"},
	"bench.history":       {"Open the history (last time)", "Abrir o histórico (última vez)"},
	"bench.noData":        {"no data", "sem dados"},
	"bench.noSwitch":      {"no data: switch sessions once", "sem dados: troque de sessão uma vez"},
	"bench.noHistory":     {"no data: press h once", "sem dados: aperte h uma vez"},
	"bench.measureErr":    {"measurement failed", "erro ao medir"},
	"bench.hookFailed":    {"hook failed", "hook falhou"},
	"bench.needSeconds":   {"-s expects a number of seconds", "-s espera um número de segundos"},
}
