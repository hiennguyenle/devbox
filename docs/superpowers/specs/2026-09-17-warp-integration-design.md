# Warp integration

## Goal

Make devbox a first-class citizen of the Warp terminal without depending on
any Warp runtime API. Two layers:

- **Outside the pane.** Warp's `+` menu lists one entry per devbox profile.
  Picking one prompts for a session name and opens a colored tab that runs
  `devbox resume`.
- **Through the pane.** The Warp tab title tracks the remote tmux session and
  the pane title (Claude Code and Codex write their task there), and Claude
  Code notifications emitted inside the remote tmux reach Warp.
- **After a reboot.** A Warp tab that was attached through devbox when the
  machine rebooted (or Warp quit or crashed) reattaches the same session
  when Warp restores the tab.

Warp has no plugin API. What Warp calls a plugin is a hook bundle for CLI
agents (`warpdotdev/claude-code-warp`) that emits an OSC 777 notification
with the title `warp://cli-agent` and a JSON body. devbox is not an agent and
Warp will never detect it as one. Everything below is files devbox writes and
bytes it sends through the terminal.

## Constraints that shaped the design (verified 2026-09-17)

- Warp's rich features (blocks, input editor, agent toolbar) do not work
  inside tmux. Warp removed its own tmux-based SSH mode in June 2026. devbox's
  model is "attach a remote tmux session", so the pane stays a plain VT.
- Tab Configs (`~/.warp/tab_configs/*.toml`) are the current launcher format
  (Launch Configurations are deprecated). Parameters are only fillable in a
  Warp modal; `warp://tab_config/<name>` cannot pass them (warp issue #12343).
- tmux drops an unknown raw OSC (e.g. OSC 777) regardless of settings, but
  forwards it when wrapped in the passthrough envelope
  `ESC P tmux; <seq with ESC doubled> ESC \` and `allow-passthrough` is on
  (tmux >= 3.3). Plain OSC 2 titles pass either way. Tested on tmux 3.6b.
- The Warp plugin writes the raw OSC 777, so it dies inside tmux today. Its
  gate `should_use_structured` needs `WARP_CLI_AGENT_PROTOCOL_VERSION` and
  `WARP_CLIENT_VERSION` in the hook's environment; sshd `AcceptEnv` on the
  test host only passes `LANG LC_* COLORTERM NO_COLOR`, so devbox must inject
  them via tmux.
- A chained `tmux new -A -s NAME \; set-option ... \; set-environment ...`
  runs the extra steps after attaching on both the create and attach-existing
  paths. Session-scoped options do not leak to other sessions. `-e KEY=VAL` is
  ignored when attaching an existing session; `set-environment` is not.
- Warp Control (`warpctrl`) exists but is not installed here, cannot submit
  input, and is macOS-only. Not used.
- Warp's session restoration brings back windows, tabs and panes with a
  fresh shell; it never re-runs commands and offers no hook. A restored tab
  keeps its `WARP_TERMINAL_SESSION_UUID`: observed on 2026-09-17 across a
  full macOS reboot plus a Warp version change (same UUID before and after,
  restored shell started three seconds after Warp launched). No environment
  variable marks a shell as restored, and none is needed.

## Decisions (locked)

- **Gate.** Warp-specific behaviour runs only when the local environment has
  `TERM_PROGRAM=WarpTerminal`. Other terminals see no change. The only
  exception is `devbox warp sync`, which is explicit and works anywhere the
  Warp directory exists.
- **Tab configs are generated, one per profile plus one for local.** File
  `devbox_<profile>.toml` (and `devbox_local.toml`) in Warp's tab_configs
  directory. First line is the marker
  `# managed by devbox warp sync — edits will be overwritten`. Sync only
  overwrites or deletes files carrying the marker. A same-named file without
  it is skipped with a warning.
- **No `title` field in generated configs.** The remote tmux title is the
  single title owner once attached. The `name` field is what the `+` menu
  shows. (If the spike shows Warp ignores OSC 2, `title` is added back and
  the remote title step is dropped. See Spike.)
- **Colors.** Profiles cycle `green, yellow, magenta, cyan, red` in sorted
  profile order. Local is always `blue`.
- **Auto-sync.** After `devbox profile` saves successfully and devbox is
  inside Warp, it runs the same sync and prints the same per-file lines.
  A sync failure there is a warning, never a failure of `devbox profile`.
- **Remote title.** Session-scoped: `set-option set-titles on` and
  `set-option set-titles-string '#S · #T'` on the attached session only.
- **Environment injection.** `set-environment WARP_CLI_AGENT_PROTOCOL_VERSION`
  and `set-environment WARP_CLIENT_VERSION` on the attached session, with the
  values copied from devbox's own environment. Only shells tmux spawns after
  the attach see them; agents started in a window or pane opened after devbox
  attached get notifications, agents already running do not. Documented as
  such. Values must match `^[A-Za-z0-9._-]+$` or the step is skipped.
- **Passthrough.** `set-option -g allow-passthrough on`, in memory on the
  running server, never written to tmux.conf. On tmux < 3.3 this prints a
  status-line error and the attach still succeeds; documented minimum for
  notifications is tmux 3.3. Title and environment work on any version.
- **Reconnect.** Every reattach re-sends the whole chain; all steps are
  idempotent.
- **Local mode** sends the same chain to local tmux.
- **Notification plumbing ships now; the plugin fix is not this repo's
  work.** Today the injected variables make the plugin emit a sequence tmux
  drops, which is the current behaviour. No regression. Notifications start
  working once `warpdotdev/claude-code-warp` wraps its sequence for tmux
  (see "Follow-up outside this repo").
- **Live record per Warp tab.** While a tmux attach runs inside Warp,
  `~/.config/devbox/warp/live/<WARP_TERMINAL_SESSION_UUID>` holds
  `<profile>\n<session>\n` (0600). Written just before the attach, removed
  when the attach returns for any reason (session ended, detached, Ctrl+C in
  a reconnect wait, or an error). It survives only when the process dies
  without returning: reboot, Warp quit, Warp crash, tab killed. The UUID must
  match `^[a-f0-9]{32}$` or nothing is recorded.
- **Restore is immediate.** `devbox warp restore` finds the record for the
  current UUID, prints `Restoring <profile>:<session>`, and runs the normal
  attach. No countdown, no prompt. Detach or Ctrl+C leaves as usual.
- **Shell hook, user-installed.** devbox never edits rc files.
  `devbox warp hook zsh|bash` prints the snippet; `devbox warp sync` ends by
  printing the one line to add to the rc file. The snippet runs only in an
  interactive shell with `TERM_PROGRAM=WarpTerminal`, a non-empty
  `WARP_TERMINAL_SESSION_UUID`, and `DEVBOX_WARP_RESTORE` not set to `0`.
- **Stale records** (a tab closed before the reboot never comes back) are
  removed by `devbox warp restore` when older than seven days, after that
  tab's own attach has returned.
- **Tracked commands:** root attach, `resume`, `new`, the picker, and local
  mode, through the single attach function. `devbox shell` is not tracked.
- **Reserved name.** `warp` joins `validate.reservedNames`.

## Command surface

```
devbox warp sync            # (re)generate Warp tab configs from config.yaml
devbox warp hook zsh|bash   # print the shell snippet that reattaches restored tabs
devbox warp restore         # reattach this Warp tab's session if it has a live record
```

`devbox warp sync` ends with:

```
To reattach devbox tabs after a reboot, add to ~/.zshrc:
  eval "$(devbox warp hook zsh)"
```

`devbox warp hook zsh` prints:

```sh
# devbox: reattach this Warp tab's session after a restart
if [[ -o interactive && "$TERM_PROGRAM" == "WarpTerminal" && -n "$WARP_TERMINAL_SESSION_UUID" && "${DEVBOX_WARP_RESTORE:-1}" != "0" ]]; then
  devbox warp restore
fi
```

The bash variant tests `$- == *i*` instead of `-o interactive`.

Output, one line per file:

```
wrote   /Users/hien/.warp/tab_configs/devbox_builder-104.toml
wrote   /Users/hien/.warp/tab_configs/devbox_local.toml
removed /Users/hien/.warp/tab_configs/devbox_old-host.toml
skipped /Users/hien/.warp/tab_configs/devbox_aaa.toml (not managed by devbox)
```

Generated file for a profile:

```toml
# managed by devbox warp sync — edits will be overwritten
name = "devbox: builder-104"
color = "green"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} -p builder-104"]

[params.session]
description = "tmux session on builder-104"
default = "work"
```

Local variant: `name = "devbox: local"`, `color = "blue"`,
`commands = ["devbox resume {{session}} --local"]`, `default = "work"`,
description `tmux session on this machine`.

Attach command inside Warp (outside Warp it stays `tmux new -A -s 'NAME'`):

```
tmux new -A -s 'api' \; set-option set-titles on \
  \; set-option set-titles-string '#S · #T' \
  \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \
  \; set-environment WARP_CLIENT_VERSION 'v0.2026.09.02.08.27.stable_01' \
  \; set-option -g allow-passthrough on
```

## Packages touched

- `internal/warp` (new)
  - `Detect() bool` — `TERM_PROGRAM == "WarpTerminal"`. A var so tests can
    force either branch.
  - `TabConfigsDir() (string, error)` — macOS `~/.warp/tab_configs`; Linux
    `${XDG_DATA_HOME:-~/.local/share}/warp-terminal/tab_configs`. Errors,
    naming the path, when the directory does not exist. Never creates it.
  - `RenderProfile(name string, p *config.Profile, color string) string` and
    `RenderLocal() string` — TOML text as above.
  - `Sync(dir string, cfg *config.Config) ([]Change, error)` — writes managed
    files (0644), removes managed files for profiles that no longer exist,
    skips unmanaged collisions. A file is "managed" when its name matches
    `devbox_*.toml` and its first line is the marker; the profile name is the
    part between `devbox_` and `.toml`, and `devbox_local.toml` is always
    kept. `Change{Action, Path}` with actions
    `wrote | removed | skipped`. `cfg` may be nil (no config yet): the local
    file is written and every managed profile file is removed, since no
    profile exists.
  - `Env() (protocol, client string)` — reads and validates the two Warp
    variables; empty when unset or invalid.
  - `SessionID() string` — `WARP_TERMINAL_SESSION_UUID` when it matches
    `^[a-f0-9]{32}$`, else empty. A var so tests can inject one.
  - `LiveDir() string` — `<config.Dir()>/warp/live`.
  - `SaveLive(dir, id, profile, session string) error`,
    `LoadLive(dir, id string) (profile, session string, err error)`,
    `ClearLive(dir, id string)`, `PruneLive(dir string, olderThan time.Duration)`.
  - `Hook(shell string) (string, error)` — the snippet for `zsh` or `bash`;
    error for anything else.
- `internal/tmux`
  - `WarpAttachSession(name, protocol, client string) string` — the chain.
    Env steps are omitted when their value is empty. Values pass through
    `quote`.
- `internal/validate` — add `"warp"` to `reservedNames`.
- `cmd`
  - `warp.go` (new): `warp` parent command with `sync`, `hook`, and
    `restore` children. `sync` prints Changes then the hook hint. `restore`
    exits 0 silently when `$TMUX` is set (a shell inside the attached tmux
    is not a restored tab), loads the record for `warp.SessionID()`, exits 0
    silently when there is none, otherwise resolves the profile (`local`
    resolves to the local executor), prints `Restoring <profile>:<session>`,
    calls `recordProfileSession`, runs `execTmuxSession`, and only after the
    attach returns prunes records older than seven days. Pruning before the
    load would race: Warp restores every tab at once, and one tab could
    delete a sibling's week-old record before that sibling reads it. A record naming
    a profile that no longer exists is removed with a warning, exit 0.
  - `root.go`: `execTmuxSession` picks `tmux.WarpAttachSession` when
    `warp.Detect()` is true, else `tmux.NewAttachSession`. When inside Warp
    and `warp.SessionID()` is non-empty it calls `SaveLive` before `Exec` and
    `ClearLive` (deferred) after it. This is the single change point for
    root, `new`, `resume`, the picker, and `restore`. It needs the profile
    name, so its signature gains `profName`.
  - `profile.go`: after `config.Save` succeeds, `if warp.Detect()` run sync
    against `warp.TabConfigsDir()`; print changes; on error print
    `warning: warp sync: <err>` to stderr.
- `README.md`: new "Warp" section (what sync writes, the `+` menu flow, the
  title, the notification prerequisites: tmux >= 3.3, agent started after
  attach, and a Warp plugin version that wraps its sequence for tmux, which
  no released version does yet; the hook line and what restore does).

## Follow-up outside this repo (not in scope)

The missing piece lives in `warpdotdev/claude-code-warp`, not in devbox.
Recorded here so whoever files it has the exact change. In
`scripts/emit-terminal-sequence.sh`, before delivery, when `$TMUX` is set
wrap the sequence:

```sh
if [ -n "${TMUX:-}" ]; then
    seq=$(printf '\033Ptmux;%s\033\\' "$(printf '%s' "$seq" | sed 's/\x1b/\x1b\x1b/g')")
fi
```

This covers both delivery paths: the `/dev/tty` write and the
`terminalSequence` JSON field. The spike only records the remote's Claude
Code version; whether Claude Code writes the wrapped string verbatim on the
JSON path is for the plugin change to verify, not devbox. The devbox README
links the PR and names the first plugin version that carries it.

## Spike (before implementation)

Assets are prepared out of tree (scratchpad `spike/`): a sample tab config
and a remote emitter script. Steps:

1. Copy `devbox_spike.toml` into `~/.warp/tab_configs/`, open it from Warp's
   `+` menu, accept the default session `warp-spike`.
2. Copy the emitter to the remote and run it inside the attached session. It
   sends, through the passthrough envelope: a bare OSC 2 title, a plain
   OSC 777 notification, and a structured `warp://cli-agent` session_start
   plus idle_prompt. It also prints the remote Claude Code version.
3. Report: tab title changed? toast appeared? tab decorated as an agent
   session?
4. `devbox kill warp-spike`; delete the spike tab config.

Outcome matrix:

| Result | Design change |
|---|---|
| Title changes | none |
| Title does not change | add `title = "{{session}} @ <profile>"` to generated configs; drop the remote title step |
| Plain toast appears | none |
| No toast | drop env injection and passthrough; notifications leave the design |
| Tab decorated | none |
| Toast but no decoration | note in README as a Warp limitation for non-detected sessions |
| Remote Claude Code >= 2.1.141 | the plugin there uses the JSON path; the PR must be verified on it |

## Error handling

- `devbox warp sync` with no Warp directory:
  `warp tab configs directory not found: <path>` (exit 1).
- Unmanaged file collision: `skipped <path> (not managed by devbox)`, exit 0.
- Missing or invalid Warp version variables: env steps silently omitted; the
  title and passthrough steps still run.
- Remote tmux errors from the chain (old tmux, unknown option) show on the
  tmux status line and never fail the attach.
- Auto-sync failure inside `devbox profile`: warning on stderr, exit 0.
- `devbox warp restore` outside Warp, inside tmux (`$TMUX` set), without a UUID, or without a record:
  exit 0, no output. Unreadable record: removed, warning, exit 0.
- `SaveLive` failure at attach: warning on stderr, attach proceeds.
- `devbox warp hook fish`: `unsupported shell "fish" (zsh, bash)`, exit 1.

## Testing

- `internal/warp`: rendering matches the fixtures above byte for byte;
  `Sync` in a temp dir writes, removes stale managed files, leaves unmanaged
  files untouched and reports them, handles nil config; `Env` rejects values
  with shell metacharacters; `Detect` follows the injected env.
- `internal/tmux`: `WarpAttachSession` produces the exact chain; omits env
  steps when values are empty; quotes a session name containing `'`.
- `internal/validate`: profile name `warp` rejected.
- `internal/warp` live records: save/load round-trip, 0600 mode, clear is
  idempotent, prune removes only files older than the cutoff, `SessionID`
  rejects a malformed value, `Hook` output contains the four guards.
- `cmd` (existing `MockExecutor`, temp config dir, `warp.Detect` forced):
  root/resume/new send the chain inside Warp and the bare command outside;
  `warp sync` output lines and the hook hint; `profile` triggers sync only
  inside Warp and survives a sync error; `execTmuxSession` writes the live
  record before `Exec` and clears it after, on both success and error;
  `warp restore` with a record sends the attach chain and prints the
  Restoring line, without a record sends nothing and prints nothing, with a
  record for a deleted profile removes it and warns.

## Out of scope

The tmux-wrapping change to Warp's Claude Code plugin (see above), Warp
Control (`warpctrl`), an MCP server or Warp skill, Codex's notify setting,
`warp://` links in `devbox list` output, Warp Preview channel paths,
Warpifying `devbox shell`, Windows.
