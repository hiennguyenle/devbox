# devbox

A CLI for managing remote tmux sessions over SSH, built to keep long-running
terminal sessions (Claude Code, Codex, builds) alive across network changes.

## Install

### From GitHub Releases

Download the latest binary for your platform from [Releases](https://github.com/hien/devbox/releases).

### From Source

```bash
go install github.com/hien/devbox@latest
```

## Setup

Configure your remote host:

```bash
devbox connect
```

This prompts for SSH host, user, and port, tests the connection, and saves the config to `~/.config/devbox/config.yaml`.

## Usage

```bash
devbox                  # attach to default session ("work")
devbox list             # list remote tmux sessions
devbox new              # create a session with a generated name
devbox new api          # create or attach to session "api"
devbox resume           # interactive picker, pre-selected on this terminal's last session
devbox resume api       # attach to session "api"
devbox kill             # interactive picker (x kills, enter kills & exits)
devbox kill api         # kill session "api"
devbox rename api backend  # rename session
devbox shell            # raw SSH, no tmux
devbox version          # print version
devbox help             # print usage
```

## Auto-Reconnect

When the SSH connection drops (laptop sleep, wifi change, moving between
office and home), devbox does not exit. It shows a reconnect spinner and
retries with backoff (2s → 5s → 10s → 30s) until connectivity returns, then
reattaches to the same tmux session. Your remote processes keep running the
whole time.

- Reconnect retries forever until you press `Ctrl+C`.
- While waiting, devbox probes the host's SSH port every 2s; the moment the
  network comes back it reconnects immediately instead of waiting out the
  backoff timer.
- A dead connection is detected within ~30s (keepalives every 10s).
- Only connection failures trigger reconnect; real errors (tmux not
  installed, bad session name) are reported immediately.
- After a stable connection, the backoff resets, so the next drop reconnects
  fast again.

## Session Tracking

Each terminal remembers which tmux session it was last attached to (keyed by
TTY). Running `devbox resume` without a name opens the session picker with
that session pre-selected. Killing a session also clears any terminal
records pointing at it.

## Requirements

- Remote host must have `tmux` installed
- Local machine must have `ssh` in PATH
