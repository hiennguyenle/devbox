# devbox

A CLI for managing remote tmux sessions over SSH.

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
devbox new api          # create or attach to session "api"
devbox resume           # reattach to this terminal's last session
devbox resume api       # attach to session "api"
devbox kill api         # kill session "api"
devbox rename api backend  # rename session
devbox shell            # raw SSH, no tmux
devbox last             # resume most recently used session (any terminal)
devbox attach           # interactive session picker
devbox help             # print usage
```

## Session Tracking

Each terminal remembers which tmux session it was last attached to. Running `devbox resume` without a name reattaches to that session automatically.

- `devbox resume` — per-terminal: resumes what THIS terminal was using
- `devbox last` — global: resumes the most recently used session across all terminals

## Requirements

- Remote host must have `tmux` installed
- Local machine must have `ssh` in PATH
