# Multi-profile support

## Goal

Let devbox manage several remote hosts as named **profiles** instead of a
single hard-coded host. Connect to a profile by name, fall back to the
last-used (per-terminal) or default profile when none is given, and manage
profiles through a `devbox profile` command.

## Decisions (locked)

- **What connecting lands you in:** a profile's own `default_session`. Bare
  `devbox` and `devbox <profile>` both SSH in and attach to that session via
  the existing auto-reconnect loop. No change to attach/reconnect behavior.
- **Setup command name:** `devbox profile` (not `config`). `connect` is kept as
  a hidden alias for muscle memory.
- **Bare `devbox` resolution:** last-used profile for this terminal if a record
  exists *and still names a real profile*, else the profile marked default.
- **Last-used scope:** per-terminal (per-TTY), mirroring how sessions are
  already tracked.
- **`-p/--profile` flag:** included. A persistent flag that targets any command
  at a specific profile without attaching first (`devbox list -p aaa`).
- **Migration name:** derived from the old host's first label, sanitized to a
  valid profile name; fallback `default`.

## Config format

`~/.config/devbox/config.yaml` becomes a map of named profiles:

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

Map keys enforce unique profile names and are cheap to look up. Listing sorts
keys alphabetically for deterministic output.

### Migration from the single-host format

The old format:

```yaml
host: dev.example.com
user: hien
port: 22
default_session: work
```

`config.Load` detects a top-level `host:` with no `profiles:` and migrates it
in memory: the profile name is the host's first label (before the first `.` or
`:`), sanitized to `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` (`dev.example.com` -> `dev`);
if sanitizing yields nothing, `default`. That profile is marked
`default_profile`, and the legacy top-level fields are cleared. Loading never
writes; the file is rewritten to the new format the next time `devbox profile`
saves.

## State files (under `~/.config/devbox/`)

| Path | Meaning | Change |
|---|---|---|
| `config.yaml` | profiles + default | restructured |
| `lastprofile/<tty>` | this terminal's last-used profile name | new |
| `sessions/<profile>/<tty>` | this terminal's last session on that profile | now profile-scoped |

Sessions move under a per-profile directory so `resume` on one box never
pre-selects a same-named session from another box, and killing a session only
clears records for that profile.

## Command surface

```
devbox                 # resolved profile (last-used this TTY, else default) -> its default_session
devbox builder-104     # attach to profile "builder-104" -> its default_session; record as last-used
devbox -p aaa list     # run a subcommand against profile "aaa" without attaching
devbox profile         # interactive: name, host, user, port, default_session
devbox profile aaa     # same, name pre-filled from the argument
devbox profile --list  # table of profiles (marker on the default)
devbox connect         # hidden alias of `devbox profile`
devbox list|new|resume|kill|rename|shell   # act on the resolved profile
devbox version|help    # unchanged
```

### Profile resolution

- **Root command** (`devbox [profile]`): `-p` flag wins; else a positional arg
  is the profile; else last-used-this-TTY-else-default.
- **Subcommands**: `-p` flag wins; else last-used-this-TTY-else-default.
  Positional args on subcommands stay session names, unchanged.

A resolved profile name must exist in the config. An unknown name errors with
`unknown profile "x". Run 'devbox profile --list'`.

### Default profile

The first profile added becomes default automatically. Adding more asks
`Set as default? [y/N]`. Re-running `devbox profile <name>` and answering yes
re-points the default. Removing a profile is out of scope (edit the YAML).

### Reserved names

A profile cannot be named after a command or built-in, so `devbox <name>` is
never ambiguous: `list`, `new`, `resume`, `kill`, `rename`, `shell`,
`version`, `profile`, `connect`, `help`, `completion`. Rejected at creation.

## Packages touched

- `internal/validate`: add `ProfileName` (charset + length + reserved-name
  rejection).
- `internal/config`:
  - `Profile` struct (host/user/port/default_session) and `Config`
    (`default_profile`, `profiles`, plus legacy fields read only for
    migration). Per-profile defaults (port 22, session `work`), validation, and
    `default_profile` existence/back-fill.
  - `lastprofile` store: `SaveLastProfile` / `GetLastProfile` keyed by TTY.
  - `session.go`: profile-scope `SaveSession` / `GetSession` /
    `DeleteSessionsByName` (add a profile argument).
- `cmd`:
  - `newClient` takes a `*config.Profile` (host/user/port) instead of the whole
    config.
  - shared `resolveProfile` helpers for root and subcommands; `-p/--profile`
    persistent flag on `rootCmd`.
  - `root.go`: `MaximumNArgs(1)`, positional profile, record last-used.
  - `profile.go` (renamed from `connect.go`): interactive add/update, `--list`,
    `connect` alias, first-profile-is-default / set-default prompt.
  - `list/new/resume/kill/rename/shell`: resolve a profile, then `newClient`.
  - `requireConfig` empty-config message points at `devbox profile`.

## Error handling

- No config / zero profiles -> `no profiles configured. Run "devbox profile"`.
- Unknown resolved profile -> unknown-profile error above.
- Reserved / invalid profile name at creation -> validation error, re-prompt
  or abort.
- Corrupt config (`default_profile` names a missing profile) -> load error.

## Testing

- `validate`: `ProfileName` valid/invalid, each reserved name rejected.
- `config`: load new format; migrate legacy format (host -> name, default set,
  legacy fields cleared); save round-trip drops legacy fields; `default_profile`
  back-fill and missing-default error; alphabetical `ProfileNames`.
- `lastprofile`: save/get, missing record.
- `sessions`: profile-scoped save/get; `DeleteSessionsByName` only touches the
  named profile.
- `cmd` (via existing `MockExecutor` + `newClient` injection): bare `devbox`
  uses last-used-else-default; `devbox <profile>` hits the right host;
  `-p` overrides; unknown profile errors; `--list` output; reserved name
  rejected; legacy config still attaches after migration.

## Out of scope

Profile removal, per-profile SSH options/identity files, importing from
`~/.ssh/config`. Revisit if needed.
