# argus

[English](README.md) · **Português**

Painel de terminal para rodar várias sessões de [Claude Code](https://claude.com/claude-code), [opencode](https://opencode.ai), [Gemini CLI](https://github.com/google-gemini/gemini-cli) e Codex em paralelo, lado a lado, sobre o tmux.

```
┌ argus ───────────────────────────┬──────────────────────────────────┐
│ api  ~/Projects                  │ > corrige o teste de upload      │
│  1 ▶ claude pes          ● 2m    │                                  │
│  2   opencode oc         ◐ 40s   │ ⏺ Vou rodar os testes primeiro.  │
│      Ajustar upload     pes  1d  │   ✓ 42 passed  ✗ 1 failed        │
│                                  │                                  │
│ site  ~/Projects/Code            │ > █                              │
│  3   claude emp ⑂        ✓ 12m   │                                  │
├──────────────────────────────────┤                                  │
│ CPU 16%  proc/min 60  3 sessões  │                                  │
└──────────────────────────────────┴──────────────────────────────────┘
```

- **Cada sessão é o próprio CLI** num terminal de verdade: memória, MCPs, skills, hooks e permissões continuam iguais.
- **As sessões abertas vêm primeiro**, com o título da conversa e a cor da conta, e o status de cada uma: ● trabalhando, ◐ esperando você, ✓ terminou. Quando o status muda, chega um aviso do macOS.
- **Conversas antigas** do Claude Code e do opencode ficam agrupadas por pasta abaixo delas: Enter (ou →) abre a pasta, Enter numa conversa a retoma.
- **Vários perfis**, por exemplo duas contas do Claude (`CLAUDE_CONFIG_DIR`) e o opencode.
- **Worktrees:** `w` abre uma sessão num branch novo, ao lado do repositório, para duas sessões não se atropelarem.
- **Desempenho medido:** sem daemon, painel sem polling, hook em shell puro. O rodapé mostra CPU e processos criados por minuto e avisa se algo disparar; `argus bench` mede tudo.

## Requisitos

macOS ou Linux, tmux 3.4+ (`brew install tmux` / `apt install tmux`), git (para worktrees). Para compilar: Go 1.26+.

## Instalação

```sh
curl -fsSL https://raw.githubusercontent.com/Marcone-Santos1/argus/main/install.sh | sh
argus install   # hooks do Claude Code, Codex e Gemini, plugin do opencode, config do tmux e ~/.config/argus/config.toml
argus           # abre o painel
```

O script baixa a última versão para o seu sistema, confere o SHA-256 e coloca o `argus` em `~/.local/bin` (`ARGUS_INSTALL_DIR` muda isso). Outros jeitos:

```sh
brew trust marcone-santos1/tap && brew install marcone-santos1/tap/argus-panel   # Homebrew
go install github.com/Marcone-Santos1/argus/cmd/argus@latest                      # pelo código
```

`argus install` acrescenta um hook aos `settings.json` do Claude Code, ao lado dos que já existem, e deixa um backup. `argus uninstall` remove tudo.

## Teclas

| Tecla | Ação |
| --- | --- |
| `↑` `↓` | seleciona; uma sessão aberta aparece à direita |
| `Enter` | entra na sessão, abre a pasta, ou retoma a conversa antiga |
| `→ ←` | abre / fecha uma pasta de conversas antigas |
| `Ctrl+\` | alterna entre a lista e a sessão (de qualquer lugar) |
| `Tab` | pula para a próxima sessão esperando você |
| `n` | nova sessão: projeto e perfil |
| `w` | nova sessão numa worktree do repositório selecionado |
| `h` | histórico completo, com busca |
| `/` | filtra a lista |
| `r` | renomeia a sessão |
| `a` | troca a conta da sessão (ex.: pessoal → trabalho), mantendo a conversa |
| `y` | responde uma sessão que espera você direto da lista: a pergunta e as opções aparecem embaixo dela |
| `?` | ajuda: todas as teclas |
| `Esc` | esconde a sessão da direita sem encerrar (ou limpa o filtro) |
| `x` | encerra a sessão (e oferece apagar a worktree) |
| `z` | tela cheia |
| `1`–`9` | vai direto para a sessão |
| `q` | fecha o painel; as sessões continuam no tmux |

## Configuração

Cada sessão aparece na lista com o texto do perfil, `claude pes` por padrão. Para trocar esse texto em todas as sessões de um perfil, use `display = "Claude pessoal"` no perfil. Para dar nome a uma sessão só, aperte `r` nela; deixar o nome vazio volta para o texto do perfil.

`~/.config/argus/config.toml`: pastas de projetos, perfis, regras de perfil por pasta, limite de sessões e quais mudanças de status geram aviso (`notify = ["waiting", "done"]`).

Se o Mac reiniciar ou o servidor do tmux cair, o próximo `argus` oferece reabrir as sessões que estavam abertas, cada uma retomando a conversa na sua conta.

## Como funciona

Um servidor tmux próprio (`tmux -L argus`) guarda as sessões. O painel fica à esquerda e troca a sessão da direita com `swap-pane`. O status vem de arquivos em `~/.local/state/argus/`, gravados pelo hook do Claude Code, Codex e Gemini (shell puro, um processo por evento) e pelo plugin do opencode (dentro do próprio opencode). O painel acompanha esses arquivos via kqueue.

A especificação completa, com regras, metas de desempenho e decisões, está em [`docs/especificacao.html`](docs/especificacao.html).

## Desenvolvimento

```
cmd/argus/          comando (main)
internal/
  bench/            argus bench
  config/           config.toml e preferências por pasta
  history/          conversas antigas (transcrições do Claude, banco do opencode)
  install/          hook, plugin e config do tmux (assets/)
  notify/           avisos do macOS
  paths/            caminhos no disco
  status/           estado de cada sessão
  sysmon/           CPU e processos por minuto
  tmux/             cliente do servidor tmux
  i18n/             textos da tela em inglês e português
  ui/               o painel
  worktree/         worktrees do git
```

```sh
make test     # testes (os de integração usam tmux e git de verdade, num servidor próprio)
make check    # gofmt, vet e testes com -race (o que o CI roda)
make install  # compila em ~/.local/bin/argus e roda argus install
make bench    # mede o desempenho com o painel aberto
```

Toda mudança precisa manter o `argus bench` dentro dos limites da especificação.

## Licença

[MIT](LICENSE).
