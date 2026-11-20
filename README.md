# argus

**English** · [Português](README.pt-BR.md)

A terminal panel for running many [Claude Code](https://claude.com/claude-code), [opencode](https://opencode.ai), [Gemini CLI](https://github.com/google-gemini/gemini-cli) and Codex sessions side by side, on top of tmux.

```
┌ argus ───────────────────────────┬──────────────────────────────────┐
│ api  ~/Projects                  │ > fix the upload test            │
│  1 ▶ claude per          ● 2m    │                                  │
│  2   opencode oc         ◐ 40s   │ ⏺ Running the tests first.       │
│      Upload tweaks      per  1d  │   ✓ 42 passed  ✗ 1 failed        │
│                                  │                                  │
│ site  ~/Projects/Code            │ > █                              │
│  3   claude wor ⑂        ✓ 12m   │                                  │
├──────────────────────────────────┤                                  │
│ CPU 16%  proc/min 60  3 sessions │                                  │
└──────────────────────────────────┴──────────────────────────────────┘
```

- **Each session is the real CLI** in a real terminal: memory, MCP servers, skills, hooks and permissions work as usual.
- **The list is grouped by folder**, with each session's status: ● working, ◐ waiting for you, ✓ done. A desktop notification arrives when the status changes.
- **Past conversations** from Claude Code and opencode show up in the list and resume with Enter.
- **Several profiles**, for example two Claude accounts (`CLAUDE_CONFIG_DIR`), opencode and Gemini.
- **Worktrees:** `w` opens a session on a new branch next to the repository, so two sessions never step on each other.
- **Measured performance:** no daemon, no polling, hooks in plain shell. The footer shows CPU and processes created per minute and warns when something runs away; `argus bench` measures it all.
- The screen follows your system language (English or Portuguese; `ARGUS_LANG=pt` or `en` forces one).

## Requirements

macOS or Linux, tmux 3.4+ (`brew install tmux` / `apt install tmux`), git (for worktrees). Building needs Go 1.26+.

## Install

```sh
brew install marcone-santos1/tap/argus-panel   # or: go install github.com/Marcone-Santos1/argus/cmd/argus@latest
argus install   # hooks for Claude Code, Codex and Gemini, the opencode plugin, tmux config and ~/.config/argus/config.toml
argus           # open the panel
```

`argus install` appends one hook to each agent's settings, next to the hooks already there, and leaves a backup. `argus uninstall` removes everything.

## Keys

| Key | Action |
| --- | --- |
| `↑` `↓` | select; an open session shows on the right |
| `Enter` | enter the session, or resume the past conversation |
| `Ctrl+\` | switch between the list and the session (from anywhere) |
| `Tab` | jump to the next session waiting for you |
| `n` | new session: project and profile |
| `w` | new session in a worktree of the selected repository |
| `h` | full history, searchable |
| `/` | filter the list |
| `r` | rename the session |
| `Esc` | hide the session on the right without closing it (or clear the filter) |
| `x` | close the session (and offer to delete its worktree) |
| `z` | zoom |
| `1`–`9` | jump to a session |
| `q` | close the panel; sessions keep running in tmux |

## Configuration

`~/.config/argus/config.toml`: project folders, profiles, per-folder profile rules, the session limit and which status changes notify you (`notify = ["waiting", "done"]`).

## How it works

A dedicated tmux server (`tmux -L argus`) holds the sessions. The panel sits on the left and swaps the session on the right with `swap-pane`. Status comes from files in `~/.local/state/argus/`, written by the Claude Code, Codex and Gemini hook (plain shell, one process per event) and by the opencode plugin (inside opencode itself). The panel watches those files with kqueue/inotify.

The full specification, with rules, performance targets and decisions, is in [`docs/especificacao.html`](docs/especificacao.html) (Portuguese).

## Development

```
cmd/argus/          the command
internal/
  bench/            argus bench
  config/           config.toml and per-folder preferences
  history/          past conversations (Claude transcripts, opencode database)
  i18n/             on-screen text in English and Portuguese
  install/          hook, plugin and tmux config (assets/)
  notify/           desktop notifications
  paths/            on-disk locations
  status/           each session's state
  sysmon/           CPU and processes per minute
  tmux/             tmux server client
  ui/               the panel
  worktree/         git worktrees
```

```sh
make test     # tests (integration tests use real tmux and git, on their own server)
make check    # gofmt, vet and tests with -race (what CI runs, on macOS and Linux)
make install  # build into ~/.local/bin/argus and run argus install
make bench    # measure performance with the panel open
```

Every change must keep `argus bench` within the limits in the specification.

Releases: push a tag (`git tag v0.1.0 && git push --tags`); CI builds the binaries and publishes the release. Then `scripts/update-tap.sh v0.1.0` updates the Homebrew formula.

## License

[MIT](LICENSE).
