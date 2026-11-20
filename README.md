# argus

Painel de terminal para rodar várias sessões de [Claude Code](https://claude.com/claude-code) e [opencode](https://opencode.ai) em paralelo, lado a lado, sobre o tmux.

```
┌ argus ───────────────────────────┬──────────────────────────────────┐
│ api  ~/Projects                  │ > corrige o teste de upload      │
│  1 ▶ claude pes          ● 2m    │                                  │
│  2   opencode oc         ◐ 40s   │ ⏺ Vou rodar os testes primeiro.  │
│      Ajustar upload     pes  1d  │   ✓ 42 passed  ✗ 1 failed        │
│                                  │                                  │
│ site  ~/Projects/Code            │ > █                              │
│  3   claude emp          ✓ 12m   │                                  │
└──────────────────────────────────┴──────────────────────────────────┘
```

- Cada sessão é o próprio CLI num terminal de verdade: memória, MCPs, skills, hooks e permissões continuam iguais.
- A lista é agrupada por pasta, com o status de cada sessão: trabalhando, esperando você, terminou.
- Conversas antigas do Claude Code e do opencode aparecem na lista e são retomadas com Enter.
- Tem vários perfis, por exemplo duas contas do Claude (`CLAUDE_CONFIG_DIR`) e o opencode.
- O desempenho é medido: nada de daemon, painel sem polling, hook em shell puro. Use `argus bench` para medir.

## Requisitos

macOS (testado no macOS 27), Go 1.24+, tmux 3.4+ (`brew install tmux`).

## Instalação

```sh
go build -o ~/.local/bin/argus .
argus install   # hook de status, config do tmux e ~/.config/argus/config.toml
argus           # abre o painel
```

`argus install` acrescenta um hook aos `settings.json` do Claude Code, ao lado dos que já existem, e deixa um backup. `argus uninstall` remove tudo.

## Teclas

| Tecla | Ação |
| --- | --- |
| `↑` `↓` | seleciona; a sessão aparece à direita |
| `Enter` | entra na sessão, ou retoma a conversa antiga |
| `Ctrl+\` | alterna entre a lista e a sessão (de qualquer lugar) |
| `n` | nova sessão: projeto e perfil |
| `h` | histórico completo, com busca |
| `Esc` | esconde a sessão da direita sem encerrar |
| `x` | encerra a sessão |
| `z` | tela cheia |
| `1`–`9` | vai direto para a sessão |
| `q` | fecha o painel; as sessões continuam no tmux |

## Como funciona

Um servidor tmux próprio (`tmux -L argus`) guarda as sessões. O painel, em Go, fica à esquerda e troca a sessão da direita com `swap-pane`. Os hooks do Claude Code gravam o status de cada sessão num arquivo em `~/.local/state/argus/`, e o painel acompanha esses arquivos via kqueue.

A especificação completa, com regras, metas de desempenho e fases, está em [`docs/especificacao.html`](docs/especificacao.html).
