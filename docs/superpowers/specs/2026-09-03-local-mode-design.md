# Local mode

## Goal

Let devbox manage tmux sessions on the **local machine** with the same
command surface it uses for remote hosts, selected by a `--local` flag:
`devbox list --local`, `devbox resume --local`, and so on. Remote behavior
is unchanged.

## Decisions (locked)

- **Selection UX:** a persistent `--local` flag on the root command,
  mutually exclusive with `-p/--profile` and with a positional profile
  argument. `devbox local` (positional, root only) is accepted as sugar for
  `devbox --local`, mirroring `devbox <profile>`.
- **Mechanism:** `local` is a built-in, reserved profile name backed by a
  local executor that implements the existing `ssh.Executor` interface
  (`Run` / `Exec`) by shelling out to `sh -c` instead of `ssh`. All
  commands already program against that interface, so they gain local
  support by resolving to the local executor.
- **Zero config:** `--local` never reads or requires `config.yaml`. Local
  mode must work on a machine with no profiles configured.
- **Reserved name:** `local` joins `validate.reservedNames`; `devbox profile`
  cannot create a profile named `local`.
- **Default session:** bare `devbox --local` attaches to session `work`
  (hardcoded, mirroring the remote convention).
- **No reconnect:** there is no transport to lose; `onDisconnect` is never
  called and the reconnect/backoff machinery does not apply locally.
- **`devbox shell --local`:** opens the user's `$SHELL` (fallback `/bin/sh`),
  mirroring raw `devbox shell` over SSH.
- **State files:** per-TTY affinity (`lastprofile/<tty>`,
  `sessions/<profile>/<tty>`) is reused as-is under the profile name
  `local`. No schema change.
- **Resume hint:** after a local session ends or is interrupted, the hint is
  `To continue this session, run devbox resume --local <name>`.

## Command surface

```
devbox --local            # attach to local session "work" (create if missing)
devbox local              # same (positional sugar, root only)
devbox list --local       # tmux list-sessions on this machine
devbox new [NAME] --local # create/attach a local session
devbox resume [NAME] --local  # attach (picker pre-selects this TTY's last local session)
devbox kill [NAME] --local    # kill a local session (picker if no name)
devbox rename OLD NEW --local
devbox shell --local      # open $SHELL, no tmux
```

`--local` combined with `-p/--profile` or a positional profile errors:
`--local cannot be combined with a profile`.

## Packages touched

- `internal/local` (new): `Executor{Verbose bool}` implementing
  `ssh.Executor`.
  - `Run(cmd)`: `sh -c cmd`, trimmed combined output.
  - `Exec(cmd, onDisconnect)`: `sh -c cmd` (or `$SHELL` when `cmd` is
    empty) with stdio inherited; `onDisconnect` ignored.
  - Verbose prints `local <cmd>` to stderr, like ssh mode prints the ssh
    invocation.
- `internal/validate`: add `"local"` to `reservedNames`.
- `cmd`:
  - root: `--local` persistent flag; `newLocalClient` injectable var
    (mirrors `newClient`); `resolveExecutor(explicit)` helper returning
    `(profileName, hostLabel, client, err)` — local branch skips config
    entirely; root accepts positional `local`.
  - `execTmuxSession` gains a `local bool` to emit `resume --local` hints.
  - `handleSSHError` gains a `local bool`: "tmux is not installed locally" /
    "local command failed" instead of host-reachability text.
  - `runAttachPicker` takes `(profName, host, defaultSession string, ...)`
    instead of `*config.Profile` so it needs no config for local.
  - `list/new/resume/kill/rename/shell` collapse their
    `requireConfig`/`resolveProfile`/`newClient` preamble into
    `resolveExecutor`.

## Error handling

- tmux missing locally: `Run` failures whose output contains
  `command not found`, and `Exec` exit code 127, map to
  `tmux is not installed locally`.
- No tmux server yet: existing `isNoServerOutput` handling works unchanged
  (`No active sessions on localhost.`).
- Any other local failure: `local command failed: <err>`.

## Testing

- `internal/local`: `Run` captures output and surfaces exit errors; `Exec`
  passes through exit codes; `Exec("")` honors `$SHELL`.
- `validate`: profile name `local` rejected.
- `cmd` (existing `MockExecutor` injected via `newLocalClient`, empty temp
  config dir): `list --local` works with no config file and runs
  `tmux list-sessions`; `--local -p x` errors; kill clears local session
  records; resume hint text carries `--local`; remote paths unaffected.

## Out of scope

Per-profile local settings (a local profile in `config.yaml`), listing the
built-in local profile in `devbox profile --list`, Windows support. Revisit
if needed.
