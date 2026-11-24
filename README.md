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
- **Open sessions come first**, named by their conversation title and colored by account, with each one's status: ● working, ◐ waiting for you, ✓ done. A desktop notification arrives when the status changes.
- **Past conversations** from Claude Code and opencode are grouped by folder below them: Enter (or →) opens a folder, Enter on a conversation resumes it. The history (h) searches titles as you type and, a moment later, the text of every conversation (≈ marks those).
- **Under each session**: its account, folder, git branch with changed files and unpushed commits, how full the context is and, once it finishes, the first line of Claude's last reply. The footer shows each Claude account's 5-hour and 7-day usage, read from Claude Code's status line (Argus installs one that prints nothing; it keeps yours if you have one).
- **Several profiles**, for example two Claude accounts (`CLAUDE_CONFIG_DIR`), opencode and Gemini.
- **Worktrees:** `w` opens a session on a new branch next to the repository, so two sessions never step on each other.
- **Measured performance:** no daemon, no polling, hooks in plain shell. The footer shows CPU and processes created per minute and warns when something runs away; `argus bench` measures it all.
- The screen follows your system language (English or Portuguese; `ARGUS_LANG=pt` or `en` forces one).

## Requirements

macOS or Linux, tmux 3.4+ (`brew install tmux` / `apt install tmux`), git (for worktrees). Building needs Go 1.26+.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/Marcone-Santos1/argus/main/install.sh | sh
argus install   # hooks for Claude Code, Codex and Gemini, the opencode plugin, tmux config and ~/.config/argus/config.toml
argus           # open the panel
```

The script downloads the latest release for your system, checks its SHA-256 and puts `argus` in `~/.local/bin` (`ARGUS_INSTALL_DIR` changes it). Other ways:

```sh
brew trust marcone-santos1/tap && brew install marcone-santos1/tap/argus-panel   # Homebrew
go install github.com/Marcone-Santos1/argus/cmd/argus@latest                      # from source
```

`argus install` appends one hook to each agent's settings, next to the hooks already there, and leaves a backup. `argus uninstall` removes everything.

## Keys

| Key | Action |
| --- | --- |
| `↑` `↓` | select; an open session shows on the right |
| `Shift+↑` `Shift+↓` (or `K` `J`) | move the session up or down the list; at the edge of its folder, the whole folder moves |
| `Enter` | enter the session, open a folder, or resume the past conversation |
| `→ ←` | open / close a folder of past conversations |
| `v` | overview: every session's screen side by side; arrows choose, Enter opens |
| `g` | today's timeline: when each session worked, waited for you or sat idle |
| `d` | diff: what changed in the selected folder, in a popup (q closes) |
| `t` | templates: start a saved session (folder, account and first message); with a session selected, save it as one |
| `< >` | narrow / widen the list (or drag the border; the width is remembered) |
| click | selects (a session shows on the right); double click enters, or resumes a conversation |
| `Ctrl+\` | switch between the list and the session (from anywhere) |
| `Tab` | jump to the next session waiting for you |
| `n` | new session: project and profile; type a path starting with `/` or `~` to pick any folder (Tab goes in) |
| `w` | new session in a worktree of the selected repository |
| `h` | full history, searchable |
| `/` | filter the list |
| `r` | rename the session (the name stays with the conversation: it comes back when you reopen it from the history) |
| `R` | restart the session on the same conversation, to pick up MCP servers, skills or settings added since it started |
| `a` | switch the session to another account (e.g. personal → work), keeping the conversation |
| `y` | answer a session waiting for you right from the list: its question and options show under it |
| `?` | help: every key |
| `Esc` | hide the session on the right without closing it (or clear the filter) |
| `x` | close the session (and offer to delete its worktree) |
| `z` | zoom |
| `1`–`9` | jump to a session |
| `q` | close the panel; sessions keep running in tmux |

## Configuration

Each session row shows its profile's text, `claude per` by default. Set `display = "Claude personal"` on a profile to change it for every session of that profile, or press `r` on a session to give just that one a name (empty brings the profile's text back).

`~/.config/argus/config.toml`: project folders, profiles, per-folder profile rules, the session limit and which status changes notify you (`notify = ["waiting", "done"]`).

A finished Claude session left untouched (not working, not on screen, nothing running behind it) for `hibernate_after` (2 hours by default) is hibernated: its process is closed to free its memory, and it stays in the list with ◌. Enter resumes the conversation where it stopped. Hibernated sessions do not count toward the session limit. `hibernate_after = "0"` turns it off.

If the machine restarts or the tmux server dies, the next `argus` offers to reopen the sessions that were open, each resuming its conversation in its account.

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
