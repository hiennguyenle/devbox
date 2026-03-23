# Devbox CLI — Design Spec

**Date:** 2026-03-23
**Status:** Draft
**Goal:** Build a Go CLI named `devbox` that connects to a configurable remote SSH host and manages tmux sessions with per-terminal session affinity.

---

## Overview

Devbox is a local CLI tool that wraps SSH and remote tmux into a simple, opinionated workflow for managing persistent development sessions on a remote machine. It replaces ad-hoc SSH + tmux commands with a clean subcommand interface, tracks which tmux session each terminal is using, and provides an interactive session picker.

## Tech Stack

- **Language:** Go 1.22+
- **CLI framework:** cobra
- **TUI (interactive picker):** bubbletea + lipgloss
- **Config format:** YAML (gopkg.in/yaml.v3)
- **SSH transport:** System `ssh` binary via `os/exec` and `syscall.Exec`
- **Build/release:** GoReleaser targeting darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
- **Distribution:** `go install` + pre-built binaries on GitHub Releases

## Architecture

### Project Structure

```
devbox/
├── cmd/
│   ├── root.go          # cobra root command (bare "devbox" = attach default session)
│   ├── connect.go       # setup SSH host interactively
│   ├── list.go          # list remote tmux sessions
│   ├── resume.go        # attach to named session or per-terminal resume
│   ├── new.go           # create/attach named session
│   ├── kill.go          # kill session
│   ├── rename.go        # rename session
│   ├── shell.go         # raw SSH, no tmux
│   ├── last.go          # resume most recent session (global)
│   └── attach.go        # interactive session picker (bubbletea)
├── internal/
│   ├── config/
│   │   └── config.go    # read/write ~/.config/devbox/config.yaml
│   ├── ssh/
│   │   ├── ssh.go       # build & exec SSH commands
│   │   └── executor.go  # Executor interface for testability
│   └── tmux/
│       └── tmux.go      # tmux command builders
├── main.go              # entry point
├── go.mod
├── go.sum
└── README.md
```

### Package Responsibilities

- **cmd/**: Cobra command definitions. Each file registers one subcommand. Responsible for argument validation and calling into `internal/` packages.
- **internal/config/**: Reads and writes `~/.config/devbox/config.yaml`. Manages session tracking files in `~/.config/devbox/sessions/`. Handles config creation, defaults, and validation.
- **internal/ssh/**: Builds SSH argument slices and executes them. Two modes — interactive (replaces process via `syscall.Exec`) and non-interactive (captures output via `exec.Command`). Never constructs shell strings; always proper argv.
- **internal/tmux/**: Pure functions that produce remote tmux command strings. No SSH awareness — just `"tmux new -A -s work"`, `"tmux list-sessions"`, etc.

## Configuration

### Config File

**Location:** `~/.config/devbox/config.yaml`

```yaml
host: builder-104
user: hien
port: 22
default_session: work
```

- Directory `~/.config/devbox/` is created automatically if missing.
- All commands except `connect` and `help` check for config existence on startup. If missing: `No host configured. Run "devbox connect" first.` → exit 1.

### `devbox connect` Flow

1. Prompt: "SSH host:" — required, no default
2. Prompt: "SSH user:" — optional, defaults to current OS user
3. Prompt: "SSH port:" — optional, defaults to 22
4. Test connection: `ssh -o ConnectTimeout=5 user@host -p port echo ok`
5. Success → save config, print confirmation
6. Failure → print error, ask to retry or save anyway

### Single Host

V1 supports one configured host at a time. Running `devbox connect` again overwrites the previous config.

## Per-Terminal Session Tracking

### Mechanism

Each terminal is identified by its TTY path (e.g., `/dev/ttys003` → stored as `ttys003`).

**Storage:** `~/.config/devbox/sessions/` directory with one file per TTY.

```
~/.config/devbox/sessions/
  ttys003    # contents: work
  ttys004    # contents: api
```

### Write Behavior

Every interactive tmux attach (`devbox`, `resume`, `new`, `last`, `attach`) resolves the current TTY and writes the session name to `~/.config/devbox/sessions/<tty_name>` before exec-ing SSH.

### Read Behavior

`devbox resume` (no NAME argument) reads the session file for the current TTY:
1. File exists and remote session exists → attach
2. File exists but remote session is gone → delete stale file, fall back to `attach` picker
3. No file → fall back to `attach` picker

### Cleanup

Stale session files are cleaned up lazily — when any read encounters a session that no longer exists on the remote, the file is deleted. No background cleanup daemon.

### `devbox last` vs `devbox resume`

- **`resume`** (no args) is **per-terminal** — "reattach to what THIS terminal was using"
- **`last`** is **global** — "reattach to the most recently used session across ALL terminals" (determined by most recently modified file in `sessions/`)

## Command Reference

### Core Commands

| Command | Mode | Behavior |
|---------|------|----------|
| `devbox` | interactive | Attach to `default_session` (default: `work`) via `tmux new -A -s`. Save TTY→session mapping. |
| `devbox connect` | interactive | Prompt host/user/port, test connection, save config. |
| `devbox list` | non-interactive | Run `tmux list-sessions`. If no server running, print friendly empty message. |
| `devbox resume [NAME]` | interactive | With NAME: attach to that session. Without NAME: look up TTY session mapping → attach. If no mapping → fall back to `attach` picker. |
| `devbox new NAME` | interactive | `tmux new -A -s NAME`. Idempotent create-or-attach. |
| `devbox kill NAME` | non-interactive | `tmux kill-session -t NAME`. No confirmation prompt. |
| `devbox rename OLD NEW` | non-interactive | `tmux rename-session -t OLD NEW`. |
| `devbox shell` | interactive | Plain `ssh -t user@host` — no tmux. |

### Additional V1 Commands

| Command | Mode | Behavior |
|---------|------|----------|
| `devbox last` | interactive | Most recently modified file in `sessions/` → attach that session. If no history → fall back to `attach` picker. |
| `devbox attach` | interactive | Fetch session list, display bubbletea picker with session names + window counts. User selects → attach. Zero sessions → offer to create default session. |

### Global Flags

- `--verbose` / `-v` — print the SSH command being executed
- `--version` — print devbox version

### Session Name Validation

Alphanumeric characters, hyphens, and underscores only. Max 50 characters. Anything else is rejected with a clear error message.

## SSH Layer

### Two Execution Modes

**Interactive (TTY replacement):** Used by commands that attach to tmux or open a shell. Calls `syscall.Exec` to replace the devbox process with SSH. This ensures proper signal handling, terminal resizing, and no zombie processes.

```
ssh -t user@host -p port "tmux new -A -s work"
```

**Non-interactive (captured output):** Used by commands that return data. Calls `exec.Command` with captured stdout/stderr.

```
ssh user@host -p port "tmux list-sessions"
```

### Command Construction

- All SSH args are built as proper argv slices — no shell string concatenation
- `-o ConnectTimeout=10` is always included
- Session names are validated before being passed to remote commands

### Error Handling

- SSH connection failure → `Cannot reach <host>. Check your connection or run "devbox connect".`
- Remote tmux not installed → detect "command not found" in stderr → `tmux is not installed on <host>.`
- Session not found (kill/rename) → forward tmux's error message as-is

## Testing Strategy

### Unit Tests

Located alongside each package (`*_test.go`):

- **internal/config/** — read/write/defaults, missing config, corrupted YAML, session file management
- **internal/ssh/** — command construction (correct args for interactive vs non-interactive, port/user handling). No actual SSH calls — verify built argv slices only.
- **internal/tmux/** — command builders produce correct remote command strings
- **cmd/** — argument validation, session name validation, missing-config detection

### Testability via Interface

```go
// internal/ssh/executor.go
type Executor interface {
    Run(args []string) (string, error)    // non-interactive, captures output
    Exec(args []string) error             // interactive, replaces process
}
```

Commands depend on this interface. Tests inject a mock executor that records calls. Production uses the real `os/exec` + `syscall.Exec` implementation.

### No Real SSH in Tests

All automated tests use mock executors. Real-host verification is a manual step.

### Build Targets

- `go test ./...` — all unit tests
- GoReleaser — cross-platform binaries for GitHub Releases
- Platforms: `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`

## Constraints & Non-Goals for V1

- Single host only (no named profiles)
- Default session name is stored in config as `default_session` (defaults to `work` if unset)
- No color output or theming beyond bubbletea defaults
- No auto-launch of tools (e.g., claude) inside sessions
- No Homebrew tap (GitHub Releases only)
- No background daemon or session monitoring
- No fuzzy search in the picker (simple arrow-key selection)

## Definition of Done

- `devbox` binary builds and runs on macOS (arm64/amd64) and Linux (amd64/arm64)
- All subcommands work against a real remote host with tmux installed
- `devbox connect` saves config and verifies connectivity
- Per-terminal session tracking works across multiple terminal tabs
- `devbox attach` shows an interactive picker via bubbletea
- `go test ./...` passes
- README documents install, setup, and all commands
- GitHub Releases has pre-built binaries via GoReleaser
