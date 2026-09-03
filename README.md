# devbox

A CLI for managing remote tmux sessions over SSH, built to keep long-running
terminal sessions (Claude Code, Codex, builds) alive across network changes.

## Install

### From MIJO Nexus (recommended)

Install the latest prebuilt binary on macOS or Linux:

```bash
curl -fsSL https://nexus.mijo.services/repository/public/devbox/install.sh | bash
```

Re-run the same command to update. The installer detects the OS and CPU,
verifies the release checksum, and installs to `~/.local/bin`. Set
`INSTALL_DIR` or `DEVBOX_VERSION` to override the destination or version.

### From GitHub Releases

Download the latest binary for your platform from [Releases](https://github.com/hiennguyenle/devbox/releases).

### With go install

```bash
go install github.com/hiennguyenle/devbox@latest
```

### From a clone

```bash
git clone https://github.com/hiennguyenle/devbox.git
cd devbox
./scripts/install.sh
```

This stamps the binary with the current git tag and installs it to
`~/.local/bin`. Set `INSTALL_DIR` to install somewhere else.

## Setup

devbox manages one or more remote hosts as named **profiles**. Add one:

```bash
devbox profile
```

This prompts for a profile name, SSH host (required), user (defaults to your
local username), port (defaults to 22), and default session, tests the
connection, and writes `~/.config/devbox/config.yaml`. If the test fails it
asks whether to save anyway. The first profile you add becomes the default;
adding more asks whether to make each the new default.

```yaml
default_profile: builder-104
profiles:
  builder-104:
    host: builder-104
    user: hien
    port: 22
    default_session: work
  aaa:
    host: 10.0.0.5
    user: root
    port: 22
    default_session: work
```

`default_session` is the session devbox attaches to when you connect to that
profile. `devbox connect` is a kept alias for `devbox profile`.

An existing single-host config (top-level `host:`) is migrated automatically:
it becomes a profile named after the host's first label.

## Local mode

Every session command also works against tmux on this machine with `--local`,
which needs no profile or config file:

```bash
devbox --local                  # attach to local session "work"
devbox list --local             # list local sessions
devbox new [NAME] --local       # create or attach
devbox resume [NAME] --local    # attach (picker if no name)
devbox kill [NAME] --local      # kill (picker if no name)
devbox rename OLD NEW --local
devbox shell --local            # open $SHELL, no tmux
```

`devbox local` is shorthand for `devbox --local`. The `--local` flag cannot
be combined with `-p` / `--profile`, and `local` is reserved as a profile name.

## Usage

```bash
devbox                  # connect to the last-used profile on this terminal, else the default
devbox aaa              # connect to profile "aaa" (attaches to its default session)
devbox profile          # add or update a profile
devbox profile aaa      # add or update profile "aaa" (name pre-filled)
devbox profile --list   # list profiles (* marks the default)
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

Every command acts on the resolved profile — the last one used on this
terminal, or the default. Use `-p` / `--profile <name>` to target a specific
profile without connecting to it first (e.g. `devbox list -p aaa`). Add `-v` /
`--verbose` to print the SSH or local command being executed.

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

Each terminal remembers, per profile (including the built-in `local` profile),
which tmux session it was last attached
to (keyed by TTY, stored under `~/.config/devbox/sessions/<profile>/`).
Running `devbox resume` without a name opens the session picker with that
session pre-selected. Killing a session also clears any terminal records
pointing at it. Each terminal also remembers its last-used profile
(`~/.config/devbox/lastprofile/`), which is what bare `devbox` reconnects to.

When a tmux-backed command exits cleanly or its SSH connection first drops,
devbox prints the command for continuing that session:

```text
To continue this session, run devbox resume api
```

## Requirements

- The selected remote host or local machine must have `tmux` installed
- Local machine must have `ssh` in PATH
- Go 1.24+ to build from source (not needed for release binaries)
- Key-based SSH auth (agent or key file). devbox reconnects unattended, so
  password auth means a prompt on every reconnect.
