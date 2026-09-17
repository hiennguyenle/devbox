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
- **Ships regardless of the upstream PR.** Today the injected variables make
  the plugin emit a sequence tmux drops, which is the current behaviour. No
  regression; it starts working when the plugin update lands.
- **Reserved name.** `warp` joins `validate.reservedNames`.

## Command surface

```
devbox warp sync     # (re)generate Warp tab configs from config.yaml
```

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
    skips unmanaged collisions. `Change{Action, Path}` with actions
    `wrote | removed | skipped`. `cfg` may be nil (no config yet): only the
    local file is written.
  - `Env() (protocol, client string)` — reads and validates the two Warp
    variables; empty when unset or invalid.
- `internal/tmux`
  - `WarpAttachSession(name, protocol, client string) string` — the chain.
    Env steps are omitted when their value is empty. Values pass through
    `quote`.
- `internal/validate` — add `"warp"` to `reservedNames`.
- `cmd`
  - `warp.go` (new): `warp` parent command with `sync` child; prints Changes.
  - `root.go`: `execTmuxSession` picks `tmux.WarpAttachSession` when
    `warp.Detect()` is true, else `tmux.NewAttachSession`. This is the single
    change point for root, `new`, `resume`, and the picker.
  - `profile.go`: after `config.Save` succeeds, `if warp.Detect()` run sync
    against `warp.TabConfigsDir()`; print changes; on error print
    `warning: warp sync: <err>` to stderr.
- `README.md`: new "Warp" section (what sync writes, the `+` menu flow, the
  title, the notification prerequisites: tmux >= 3.3, agent started after
  attach, plugin version with the tmux fix).

## Upstream PR (warpdotdev/claude-code-warp)

In `scripts/emit-terminal-sequence.sh`, before delivery, when `$TMUX` is set
wrap the sequence:

```sh
if [ -n "${TMUX:-}" ]; then
    seq=$(printf '\033Ptmux;%s\033\\' "$(printf '%s' "$seq" | sed 's/\x1b/\x1b\x1b/g')")
fi
```

This covers both delivery paths: the `/dev/tty` write and the
`terminalSequence` JSON field (Claude Code writes that string verbatim; the
spike confirms this on the remote's Claude Code version). The devbox README
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
| No toast | drop env injection and passthrough; notifications leave the design; PR still worth filing |
| Tab decorated | none |
| Toast but no decoration | note in README as a Warp limitation for non-detected sessions |
| Remote Claude Code >= 2.1.141 | PR must be verified on the JSON path too |

## Error handling

- `devbox warp sync` with no Warp directory:
  `warp tab configs directory not found: <path>` (exit 1).
- Unmanaged file collision: `skipped <path> (not managed by devbox)`, exit 0.
- Missing or invalid Warp version variables: env steps silently omitted; the
  title and passthrough steps still run.
- Remote tmux errors from the chain (old tmux, unknown option) show on the
  tmux status line and never fail the attach.
- Auto-sync failure inside `devbox profile`: warning on stderr, exit 0.

## Testing

- `internal/warp`: rendering matches the fixtures above byte for byte;
  `Sync` in a temp dir writes, removes stale managed files, leaves unmanaged
  files untouched and reports them, handles nil config; `Env` rejects values
  with shell metacharacters; `Detect` follows the injected env.
- `internal/tmux`: `WarpAttachSession` produces the exact chain; omits env
  steps when values are empty; quotes a session name containing `'`.
- `internal/validate`: profile name `warp` rejected.
- `cmd` (existing `MockExecutor`, temp config dir, `warp.Detect` forced):
  root/resume/new send the chain inside Warp and the bare command outside;
  `warp sync` output lines; `profile` triggers sync only inside Warp and
  survives a sync error.

## Out of scope

Warp Control (`warpctrl`), an MCP server or Warp skill, Codex's notify
setting, `warp://` links in `devbox list` output, Warp Preview channel
paths, Warpifying `devbox shell`, Windows.
