# Warp Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make devbox a first-class citizen of the Warp terminal: generated tab configs in Warp's `+` menu, live tab titles and notification plumbing through remote tmux, and automatic reattach of restored Warp tabs after a reboot.

**Architecture:** A new `internal/warp` package owns Warp detection, tab-config generation, live-record files, and the shell hook. `internal/tmux` gains one builder for the chained attach-and-configure command. The `cmd` layer wires it in at a single attach function (`execTmuxSession`) plus a new `warp` subcommand with `sync`, `hook`, and `restore`. Nothing talks to the Warp app process; everything is files devbox writes and bytes it sends through the terminal.

**Tech Stack:** Go 1.24, cobra, standard library only (no new dependencies). Tests use `testing`, `t.TempDir`, `t.Setenv`, and the existing `ssh.MockExecutor`.

**Spec:** `docs/superpowers/specs/2026-09-17-warp-integration-design.md`

## Global Constraints

- Warp-specific behaviour runs only when `TERM_PROGRAM=WarpTerminal` in devbox's own environment. `devbox warp sync` is the one exception and works wherever the Warp directory exists.
- Marker line, first line of every generated file, verbatim: `# managed by devbox warp sync — edits will be overwritten` (note the em dash).
- Generated files are `devbox_<profile>.toml` and `devbox_local.toml`, mode 0644. Only files carrying the marker may be overwritten or removed.
- Profile colors cycle `green, yellow, magenta, cyan, red` in sorted profile order. Local is always `blue`. No `title` field.
- Attach chain inside Warp, exactly: `tmux new -A -s 'NAME' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION 'P' \; set-environment WARP_CLIENT_VERSION 'C' \; set-option -g allow-passthrough on`. Env steps are omitted when their value is empty. Outside Warp the command stays `tmux new -A -s 'NAME'`.
- Env values must match `^[A-Za-z0-9._-]+$` or are treated as empty. Warp session id must match `^[a-f0-9]{32}$` or is treated as empty.
- Live records: `<config.Dir()>/warp/live/<uuid>`, content `<profile>\n<session>\n`, mode 0600. Pruned by `devbox warp restore` when older than 7 days, after loading the current record.
- devbox never edits rc files. `devbox warp restore` is silent (exit 0, no output) when there is nothing to do.
- Documented minimum for notifications: tmux >= 3.3 on the remote, agent started in a window opened after attach, and a Warp plugin version that wraps for tmux (none released yet).
- Existing behaviour outside Warp must not change; the full existing test suite must stay green after every task.

## Pre-flight: spike results

The user is running the spike described in the spec. Its outcome can adjust two tasks:

- If the Warp tab title did **not** change: in Task 2 add `title = "{{session}} @ <name>"` (and `title = "{{session}} @ local"`) as the line after `name = ...` in the rendered TOML, and in Task 5 drop the two `set-option set-titles*` steps.
- If **no toast** appeared: in Task 5 drop the `set-environment` and `allow-passthrough` steps, and in Task 6 stop calling `warp.Env()`.

If results are not in when a task starts, implement as written; both adjustments are one-line edits later.

## File structure

| File | Responsibility |
|---|---|
| `internal/warp/detect.go` | Environment facts: `Detect`, `SessionID`, `Env`, `TabConfigsDir` |
| `internal/warp/tabconfig.go` | Marker, colors, `RenderProfile`, `RenderLocal`, `Sync`, `Change` |
| `internal/warp/live.go` | Live-record files: `LiveDir`, `SaveLive`, `LoadLive`, `ClearLive`, `PruneLive` |
| `internal/warp/hook.go` | `Hook(shell)` shell snippet |
| `internal/tmux/tmux.go` | `WarpAttachSession` chain builder |
| `internal/validate/validate.go` | `warp` reserved as a profile name |
| `cmd/root.go` | `execTmuxSession` picks the chain and tracks the live record |
| `cmd/warp.go` | `devbox warp sync|hook|restore` |
| `cmd/profile.go` | auto-sync after save inside Warp |
| `README.md` | Warp section |

---

### Task 1: Warp environment facts (`internal/warp/detect.go`)

**Files:**
- Create: `internal/warp/detect.go`
- Test: `internal/warp/detect_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces:
  - `var Detect = func() bool` — true when `TERM_PROGRAM == "WarpTerminal"`.
  - `var SessionID = func() string` — `WARP_TERMINAL_SESSION_UUID` when it matches `^[a-f0-9]{32}$`, else `""`.
  - `func Env() (protocol, client string)` — validated `WARP_CLI_AGENT_PROTOCOL_VERSION` and `WARP_CLIENT_VERSION`, each `""` when unset or invalid.
  - `func TabConfigsDir() (string, error)` — Warp's tab_configs directory; error `warp tab configs directory not found: <path>` when it does not exist.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warp/detect_test.go
package warp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "WarpTerminal")
	if !Detect() {
		t.Error("Detect() = false inside Warp, want true")
	}
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if Detect() {
		t.Error("Detect() = true outside Warp, want false")
	}
}

func TestSessionID_AcceptsWarpUUID(t *testing.T) {
	t.Setenv("WARP_TERMINAL_SESSION_UUID", "aca3a00ac45440f19afea15ac826acd0")
	if got := SessionID(); got != "aca3a00ac45440f19afea15ac826acd0" {
		t.Errorf("SessionID() = %q, want the env value", got)
	}
}

func TestSessionID_RejectsMalformed(t *testing.T) {
	bad := []string{"", "ACA3A00AC45440F19AFEA15AC826ACD0", "../etc/passwd", "abc", "aca3a00ac45440f19afea15ac826acd0x"}
	for _, v := range bad {
		t.Setenv("WARP_TERMINAL_SESSION_UUID", v)
		if got := SessionID(); got != "" {
			t.Errorf("SessionID() with %q = %q, want empty", v, got)
		}
	}
}

func TestEnv_ReadsAndValidates(t *testing.T) {
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.09.02.08.27.stable_01")
	p, c := Env()
	if p != "1" || c != "v0.2026.09.02.08.27.stable_01" {
		t.Errorf("Env() = %q, %q; want 1, v0.2026.09.02.08.27.stable_01", p, c)
	}
}

func TestEnv_RejectsShellMetacharacters(t *testing.T) {
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1; rm -rf /")
	t.Setenv("WARP_CLIENT_VERSION", "")
	p, c := Env()
	if p != "" || c != "" {
		t.Errorf("Env() = %q, %q; want both empty", p, c)
	}
}

func TestTabConfigsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	want := filepath.Join(home, ".warp", "tab_configs")
	if runtime.GOOS != "darwin" {
		want = filepath.Join(home, ".local", "share", "warp-terminal", "tab_configs")
	}

	if _, err := TabConfigsDir(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("missing dir: err = %v, want error naming %s", err, want)
	}
	if err := os.MkdirAll(want, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := TabConfigsDir()
	if err != nil || got != want {
		t.Errorf("TabConfigsDir() = %q, %v; want %q", got, err, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warp/ -run 'TestDetect|TestSessionID|TestEnv|TestTabConfigsDir' -v`
Expected: build failure, `undefined: Detect` (package has no non-test files yet).

- [ ] **Step 3: Write the implementation**

```go
// internal/warp/detect.go
// Package warp integrates devbox with the Warp terminal: it detects Warp,
// generates Warp tab configs, tracks which Warp tab is attached to which
// session, and prints the shell hook that reattaches restored tabs.
package warp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

// Detect reports whether devbox is running inside the Warp terminal. A var
// so tests can force either branch without touching the environment.
var Detect = func() bool {
	return os.Getenv("TERM_PROGRAM") == "WarpTerminal"
}

var sessionIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// SessionID returns the Warp session UUID of this tab, or "" when unset or
// malformed. Warp keeps the UUID stable across restarts and reboots, which
// is what lets a restored tab find its own live record. A var so tests can
// inject one.
var SessionID = func() string {
	id := os.Getenv("WARP_TERMINAL_SESSION_UUID")
	if !sessionIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// envValuePattern keeps the values devbox forwards into a remote tmux
// command to a single safe word (they are single-quoted anyway; this is
// defence in depth).
var envValuePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Env returns Warp's CLI-agent protocol version and client version, each ""
// when unset or invalid. They are forwarded into the remote tmux session so
// Warp's Claude Code plugin there sees the same values a local shell would.
func Env() (protocol, client string) {
	protocol = os.Getenv("WARP_CLI_AGENT_PROTOCOL_VERSION")
	if !envValuePattern.MatchString(protocol) {
		protocol = ""
	}
	client = os.Getenv("WARP_CLIENT_VERSION")
	if !envValuePattern.MatchString(client) {
		client = ""
	}
	return protocol, client
}

// TabConfigsDir returns Warp's tab_configs directory for this platform. It
// never creates the directory: a missing one means Warp is not installed
// (or has never run), and that is an error the user should see.
func TabConfigsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	var dir string
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(home, ".warp", "tab_configs")
	} else {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "warp-terminal", "tab_configs")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("warp tab configs directory not found: %s", dir)
	}
	return dir, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warp/ -v`
Expected: all six tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warp/detect.go internal/warp/detect_test.go
git commit -m "feat(warp): detect Warp, session id, env, and tab configs dir"
```

---

### Task 2: Tab config generation and sync (`internal/warp/tabconfig.go`)

**Files:**
- Create: `internal/warp/tabconfig.go`
- Test: `internal/warp/tabconfig_test.go`

**Interfaces:**
- Consumes: `config.Config`, `config.Profile`, `(*config.Config).ProfileNames()` from `internal/config`.
- Produces:
  - `const Marker = "# managed by devbox warp sync — edits will be overwritten"`
  - `const ActionWrote = "wrote"`, `ActionRemoved = "removed"`, `ActionSkipped = "skipped"`
  - `type Change struct { Action, Path string }`
  - `func ColorFor(i int) string`
  - `func RenderProfile(name string, p *config.Profile, color string) string`
  - `func RenderLocal() string`
  - `func Sync(dir string, cfg *config.Config) ([]Change, error)` — `cfg` may be nil.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warp/tabconfig_test.go
package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiennguyenle/devbox/internal/config"
)

func TestRenderProfile(t *testing.T) {
	got := RenderProfile("builder-104", &config.Profile{Host: "builder-104", DefaultSession: "work"}, "green")
	want := Marker + `
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
`
	if got != want {
		t.Errorf("RenderProfile =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderLocal(t *testing.T) {
	got := RenderLocal()
	want := Marker + `
name = "devbox: local"
color = "blue"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} --local"]

[params.session]
description = "tmux session on this machine"
default = "work"
`
	if got != want {
		t.Errorf("RenderLocal =\n%s\nwant\n%s", got, want)
	}
}

func TestColorFor_CyclesFiveColors(t *testing.T) {
	want := []string{"green", "yellow", "magenta", "cyan", "red", "green"}
	for i, w := range want {
		if got := ColorFor(i); got != w {
			t.Errorf("ColorFor(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestSync_WritesProfilesThenLocal(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DefaultProfile: "b", Profiles: map[string]*config.Profile{
		"b": {Host: "hb", DefaultSession: "work"},
		"a": {Host: "ha", DefaultSession: "main"},
	}}

	changes, err := Sync(dir, cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	wantPaths := []string{
		filepath.Join(dir, "devbox_a.toml"),
		filepath.Join(dir, "devbox_b.toml"),
		filepath.Join(dir, "devbox_local.toml"),
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %+v, want 3", changes)
	}
	for i, c := range changes {
		if c.Action != ActionWrote || c.Path != wantPaths[i] {
			t.Errorf("changes[%d] = %+v, want wrote %s", i, c, wantPaths[i])
		}
	}
	// sorted profile order decides the color: a=green, b=yellow
	if data, _ := os.ReadFile(wantPaths[0]); string(data) != RenderProfile("a", cfg.Profiles["a"], "green") {
		t.Errorf("devbox_a.toml content mismatch:\n%s", data)
	}
	if data, _ := os.ReadFile(wantPaths[1]); string(data) != RenderProfile("b", cfg.Profiles["b"], "yellow") {
		t.Errorf("devbox_b.toml content mismatch:\n%s", data)
	}
	if data, _ := os.ReadFile(wantPaths[2]); string(data) != RenderLocal() {
		t.Errorf("devbox_local.toml content mismatch:\n%s", data)
	}
	if info, _ := os.Stat(wantPaths[0]); info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want 0644", info.Mode().Perm())
	}
}

func TestSync_NilConfigWritesLocalAndRemovesStaleManaged(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "devbox_old.toml")
	if err := os.WriteFile(stale, []byte(Marker+"\nname = \"x\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := Sync(dir, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	want := []Change{
		{ActionWrote, filepath.Join(dir, "devbox_local.toml")},
		{ActionRemoved, stale},
	}
	if len(changes) != 2 || changes[0] != want[0] || changes[1] != want[1] {
		t.Errorf("changes = %+v, want %+v", changes, want)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale managed file should be removed")
	}
}

func TestSync_SkipsUnmanagedCollision(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "devbox_a.toml")
	hand := "name = \"hand written\"\n"
	if err := os.WriteFile(mine, []byte(hand), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Profiles: map[string]*config.Profile{"a": {Host: "h", DefaultSession: "work"}}}

	changes, err := Sync(dir, cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(changes) < 1 || changes[0] != (Change{ActionSkipped, mine}) {
		t.Errorf("changes[0] = %+v, want skipped %s", changes, mine)
	}
	if data, _ := os.ReadFile(mine); string(data) != hand {
		t.Error("unmanaged file must not be overwritten")
	}
}

func TestSync_LeavesUnmanagedStaleFilesAlone(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "devbox_other.toml")
	if err := os.WriteFile(other, []byte("name = \"hand written\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("unmanaged file with devbox_ prefix must survive sync")
	}
}

func TestSync_MissingDirErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := Sync(missing, nil)
	if err == nil || !strings.Contains(err.Error(), "warp tab configs directory not found: "+missing) {
		t.Errorf("err = %v, want directory-not-found naming %s", err, missing)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warp/ -run 'TestRender|TestColorFor|TestSync' -v`
Expected: build failure, `undefined: RenderProfile` (and friends).

- [ ] **Step 3: Write the implementation**

```go
// internal/warp/tabconfig.go
package warp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiennguyenle/devbox/internal/config"
)

// Marker is the first line of every file devbox writes into Warp's
// tab_configs directory. Sync only ever overwrites or removes files that
// carry it, so hand-written configs are never touched.
const Marker = "# managed by devbox warp sync — edits will be overwritten"

const (
	ActionWrote   = "wrote"
	ActionRemoved = "removed"
	ActionSkipped = "skipped"
)

// Change is one thing Sync did (or refused to do) to one file.
type Change struct {
	Action string
	Path   string
}

const (
	filePrefix = "devbox_"
	fileSuffix = ".toml"
	localKey   = "local"
	localColor = "blue"
)

// profileColors are cycled in sorted profile order. Blue is reserved for
// local; black and white make poor tab markers.
var profileColors = []string{"green", "yellow", "magenta", "cyan", "red"}

// ColorFor returns the tab color for the i-th profile in sorted order.
func ColorFor(i int) string {
	return profileColors[i%len(profileColors)]
}

// RenderProfile renders the tab config for one profile. Profile names, hosts
// and session names are validated to a character set that needs no TOML
// escaping. The session parameter is filled in Warp's modal on open, since
// warp://tab_config URIs cannot carry parameters.
func RenderProfile(name string, p *config.Profile, color string) string {
	return fmt.Sprintf(`%s
name = "devbox: %s"
color = "%s"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} -p %s"]

[params.session]
description = "tmux session on %s"
default = "%s"
`, Marker, name, color, name, p.Host, p.DefaultSession)
}

// RenderLocal renders the tab config for the built-in local profile.
func RenderLocal() string {
	return fmt.Sprintf(`%s
name = "devbox: local"
color = "%s"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} --local"]

[params.session]
description = "tmux session on this machine"
default = "work"
`, Marker, localColor)
}

// Sync makes dir hold exactly one managed file per configured profile plus
// the local one. cfg may be nil (no config yet): then only the local file is
// wanted and every managed profile file is stale. Returns the changes in
// order: writes (profiles sorted, then local), then removals.
func Sync(dir string, cfg *config.Config) ([]Change, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("warp tab configs directory not found: %s", dir)
	}

	var order []string
	want := map[string]string{}
	if cfg != nil {
		for i, name := range cfg.ProfileNames() {
			order = append(order, name)
			want[name] = RenderProfile(name, cfg.Profiles[name], ColorFor(i))
		}
	}
	order = append(order, localKey)
	want[localKey] = RenderLocal()

	var changes []Change
	for _, key := range order {
		path := filepath.Join(dir, filePrefix+key+fileSuffix)
		if managed, exists := isManaged(path); exists && !managed {
			changes = append(changes, Change{ActionSkipped, path})
			continue
		}
		if err := os.WriteFile(path, []byte(want[key]), 0644); err != nil {
			return changes, err
		}
		changes = append(changes, Change{ActionWrote, path})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return changes, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileSuffix) {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileSuffix)
		if _, wanted := want[key]; wanted {
			continue
		}
		path := filepath.Join(dir, n)
		if managed, _ := isManaged(path); !managed {
			continue
		}
		if err := os.Remove(path); err != nil {
			return changes, err
		}
		changes = append(changes, Change{ActionRemoved, path})
	}
	return changes, nil
}

// isManaged reports whether path exists and, if so, whether its first line
// is the Marker.
func isManaged(path string) (managed, exists bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text() == Marker, true
	}
	return false, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warp/ -v`
Expected: all tests PASS (Task 1's six plus these eight).

- [ ] **Step 5: Commit**

```bash
git add internal/warp/tabconfig.go internal/warp/tabconfig_test.go
git commit -m "feat(warp): render and sync marker-owned tab configs"
```

---

### Task 3: Live records (`internal/warp/live.go`)

**Files:**
- Create: `internal/warp/live.go`
- Test: `internal/warp/live_test.go`

**Interfaces:**
- Consumes: `config.Dir()`, `config.SetDirOverride` from `internal/config`.
- Produces:
  - `func LiveDir() string` — `<config.Dir()>/warp/live`
  - `func SaveLive(dir, id, profile, session string) error`
  - `func LoadLive(dir, id string) (profile, session string, err error)` — a missing record returns an error satisfying `errors.Is(err, fs.ErrNotExist)`; a malformed one returns a different error.
  - `func ClearLive(dir, id string)` — idempotent.
  - `func PruneLive(dir string, olderThan time.Duration)` — removes records with mtime older than the cutoff.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warp/live_test.go
package warp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiennguyenle/devbox/internal/config"
)

func TestLive_SaveLoadClear(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "live") // SaveLive must create it
	id := "aca3a00ac45440f19afea15ac826acd0"

	if err := SaveLive(dir, id, "builder-104", "api"); err != nil {
		t.Fatalf("SaveLive: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, id)); string(data) != "builder-104\napi\n" {
		t.Errorf("content = %q, want profile and session lines", data)
	}

	p, s, err := LoadLive(dir, id)
	if err != nil || p != "builder-104" || s != "api" {
		t.Errorf("LoadLive = %q, %q, %v; want builder-104, api, nil", p, s, err)
	}

	ClearLive(dir, id)
	ClearLive(dir, id) // second clear is a no-op, never an error
	if _, _, err := LoadLive(dir, id); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after clear: err = %v, want not-exist", err)
	}
}

func TestLoadLive_MalformedIsNotNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc"), []byte("only-one-line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadLive(dir, "abc")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want a malformed-record error distinct from not-exist", err)
	}
}

func TestPruneLive_RemovesOnlyOldRecords(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"old", "new"} {
		if err := SaveLive(dir, id, "p", "s"); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old"), past, past); err != nil {
		t.Fatal(err)
	}

	PruneLive(dir, 7*24*time.Hour)

	if _, err := os.Stat(filepath.Join(dir, "old")); !os.IsNotExist(err) {
		t.Error("old record should be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); err != nil {
		t.Error("fresh record should survive")
	}
}

func TestPruneLive_MissingDirIsNoop(t *testing.T) {
	PruneLive(filepath.Join(t.TempDir(), "nope"), time.Hour) // must not panic
}

func TestLiveDir_UnderConfigDir(t *testing.T) {
	config.SetDirOverride("/cfg")
	t.Cleanup(func() { config.SetDirOverride("") })
	if got := LiveDir(); got != filepath.Join("/cfg", "warp", "live") {
		t.Errorf("LiveDir() = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warp/ -run 'TestLive|TestLoadLive|TestPruneLive' -v`
Expected: build failure, `undefined: SaveLive`.

- [ ] **Step 3: Write the implementation**

```go
// internal/warp/live.go
package warp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hiennguyenle/devbox/internal/config"
)

// LiveDir holds one record per Warp tab that currently has a devbox tmux
// attach running: <config dir>/warp/live/<WARP_TERMINAL_SESSION_UUID>.
// A record that outlives its process (reboot, Warp quit, crash) is what
// `devbox warp restore` uses to reattach the restored tab.
func LiveDir() string {
	return filepath.Join(config.Dir(), "warp", "live")
}

// SaveLive records that the Warp tab id is attached to session on profile.
func SaveLive(dir, id, profile, session string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, id), []byte(profile+"\n"+session+"\n"), 0600)
}

// LoadLive reads the record for id. A missing record returns an error that
// satisfies errors.Is(err, fs.ErrNotExist); a malformed one does not, so
// callers can tell "nothing to restore" from "remove this junk".
func LoadLive(dir, id string) (profile, session string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, id))
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", fmt.Errorf("malformed live record %s", id)
	}
	return lines[0], lines[1], nil
}

// ClearLive removes the record for id. Idempotent.
func ClearLive(dir, id string) {
	os.Remove(filepath.Join(dir, id))
}

// PruneLive removes records whose modification time is older than
// olderThan. A tab closed before a reboot never comes back, so its record
// would otherwise live forever.
func PruneLive(dir string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warp/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warp/live.go internal/warp/live_test.go
git commit -m "feat(warp): live records keyed by Warp session id"
```

---

### Task 4: Shell hook snippet (`internal/warp/hook.go`)

**Files:**
- Create: `internal/warp/hook.go`
- Test: `internal/warp/hook_test.go`

**Interfaces:**
- Produces: `func Hook(shell string) (string, error)` — snippet for `zsh` or `bash`; error `unsupported shell "<shell>" (zsh, bash)` otherwise.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warp/hook_test.go
package warp

import (
	"strings"
	"testing"
)

func TestHook_Zsh(t *testing.T) {
	got, err := Hook("zsh")
	if err != nil {
		t.Fatal(err)
	}
	want := `# devbox: reattach this Warp tab's session after a restart
if [[ -o interactive && "$TERM_PROGRAM" == "WarpTerminal" && -n "$WARP_TERMINAL_SESSION_UUID" && "${DEVBOX_WARP_RESTORE:-1}" != "0" ]]; then
  devbox warp restore
fi
`
	if got != want {
		t.Errorf("Hook(zsh) =\n%s\nwant\n%s", got, want)
	}
}

func TestHook_BashUsesInteractiveFlagTest(t *testing.T) {
	got, err := Hook("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, guard := range []string{
		`$- == *i*`,
		`"$TERM_PROGRAM" == "WarpTerminal"`,
		`-n "$WARP_TERMINAL_SESSION_UUID"`,
		`"${DEVBOX_WARP_RESTORE:-1}" != "0"`,
		"devbox warp restore",
	} {
		if !strings.Contains(got, guard) {
			t.Errorf("Hook(bash) missing %q:\n%s", guard, got)
		}
	}
	if strings.Contains(got, "-o interactive") {
		t.Error("bash snippet must not use the zsh-only -o interactive test")
	}
}

func TestHook_UnsupportedShell(t *testing.T) {
	_, err := Hook("fish")
	if err == nil || err.Error() != `unsupported shell "fish" (zsh, bash)` {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warp/ -run TestHook -v`
Expected: build failure, `undefined: Hook`.

- [ ] **Step 3: Write the implementation**

```go
// internal/warp/hook.go
package warp

import "fmt"

// Hook returns the rc-file snippet that reattaches a restored Warp tab. It
// runs only in an interactive shell, inside Warp, with a session id, and
// with DEVBOX_WARP_RESTORE not set to 0. devbox prints it; the user pastes
// it. devbox never edits rc files.
func Hook(shell string) (string, error) {
	var interactive string
	switch shell {
	case "zsh":
		interactive = "-o interactive"
	case "bash":
		interactive = "$- == *i*"
	default:
		return "", fmt.Errorf("unsupported shell %q (zsh, bash)", shell)
	}
	return fmt.Sprintf(`# devbox: reattach this Warp tab's session after a restart
if [[ %s && "$TERM_PROGRAM" == "WarpTerminal" && -n "$WARP_TERMINAL_SESSION_UUID" && "${DEVBOX_WARP_RESTORE:-1}" != "0" ]]; then
  devbox warp restore
fi
`, interactive), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warp/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warp/hook.go internal/warp/hook_test.go
git commit -m "feat(warp): shell hook snippet for restored tabs"
```

---

### Task 5: Chained attach command (`internal/tmux`)

**Files:**
- Modify: `internal/tmux/tmux.go` (append after `NewAttachSession`)
- Test: `internal/tmux/tmux_test.go` (append)

**Interfaces:**
- Consumes: `quote`, `NewAttachSession` already in `internal/tmux/tmux.go`.
- Produces: `func WarpAttachSession(name, protocol, client string) string`.

- [ ] **Step 1: Write the failing tests** (append to `internal/tmux/tmux_test.go`)

```go
func TestWarpAttachSession_FullChain(t *testing.T) {
	got := WarpAttachSession("api", "1", "v0.2026.09.02.08.27.stable_01")
	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \; set-environment WARP_CLIENT_VERSION 'v0.2026.09.02.08.27.stable_01' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_OmitsEmptyEnvSteps(t *testing.T) {
	got := WarpAttachSession("api", "", "")
	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_QuotesSessionName(t *testing.T) {
	got := WarpAttachSession("it's", "", "")
	if want := `tmux new -A -s 'it'\''s' \; `; !strings.HasPrefix(got, want) {
		t.Errorf("WarpAttachSession = %q, want prefix %q", got, want)
	}
}
```

Add `"strings"` to the test file's imports: change `import "testing"` to

```go
import (
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tmux/ -run TestWarpAttachSession -v`
Expected: build failure, `undefined: WarpAttachSession`.

- [ ] **Step 3: Write the implementation** (append to `internal/tmux/tmux.go`)

```go
// WarpAttachSession is NewAttachSession followed by the configuration a
// Warp tab needs, chained with tmux's ";" separator so it runs after the
// attach on both the create and attach-existing paths:
//
//   - the session's outward title becomes "session · pane title", so the
//     Warp tab shows the session and whatever Claude Code or Codex wrote
//     into the pane title (session-scoped; other sessions keep tmux.conf);
//   - Warp's version variables are injected into the session environment
//     so Warp's Claude Code plugin in windows opened later can emit
//     notifications (omitted when empty);
//   - allow-passthrough is turned on in memory so those notifications can
//     traverse tmux (tmux >= 3.3; older servers print a status-line error
//     and the attach still succeeds).
func WarpAttachSession(name, protocol, client string) string {
	var b strings.Builder
	b.WriteString(NewAttachSession(name))
	b.WriteString(` \; set-option set-titles on`)
	b.WriteString(` \; set-option set-titles-string '#S · #T'`)
	if protocol != "" {
		b.WriteString(` \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION ` + quote(protocol))
	}
	if client != "" {
		b.WriteString(` \; set-environment WARP_CLIENT_VERSION ` + quote(client))
	}
	b.WriteString(` \; set-option -g allow-passthrough on`)
	return b.String()
}
```

`internal/tmux/tmux.go` already imports `strings`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmux/ -v`
Expected: all PASS, including the pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/tmux.go internal/tmux/tmux_test.go
git commit -m "feat(tmux): chained attach command for Warp tabs"
```

---

### Task 6: Attach inside Warp and track the live record (`cmd/root.go`)

**Files:**
- Modify: `cmd/root.go` (`runRoot`, `execTmuxSession`)
- Modify: `cmd/attach.go:144` and `cmd/attach.go:174` (call sites)
- Modify: `cmd/new.go:43`, `cmd/resume.go:33` (call sites)
- Test: `cmd/cmd_test.go`

**Interfaces:**
- Consumes: `warp.Detect`, `warp.SessionID`, `warp.Env`, `warp.LiveDir`, `warp.SaveLive`, `warp.ClearLive` (Tasks 1, 3); `tmux.WarpAttachSession` (Task 5).
- Produces: `func execTmuxSession(client sshpkg.Executor, profName, host, sessionName string) error`. The former `local bool` parameter is gone; it is derived from `profName == localProfileName`. Task 8 calls this signature.

- [ ] **Step 1: Add a TestMain that keeps every cmd test outside Warp by default**

The developer's own terminal is Warp, so without this the real environment would leak into tests and write live records under `~/.config/devbox`. Add to `cmd/cmd_test.go`, after the imports (add `"github.com/hiennguyenle/devbox/internal/warp"` to the import block):

```go
// TestMain pins every test outside Warp. Tests that need Warp call
// forceWarp(t, true, id).
func TestMain(m *testing.M) {
	warp.Detect = func() bool { return false }
	warp.SessionID = func() string { return "" }
	os.Unsetenv("TMUX") // same results whether or not `go test` itself runs inside tmux
	os.Exit(m.Run())
}

// forceWarp makes devbox believe it is (or is not) inside Warp with the
// given session id, for the duration of one test.
func forceWarp(t *testing.T, inside bool, sessionID string) {
	t.Helper()
	origDetect, origID := warp.Detect, warp.SessionID
	warp.Detect = func() bool { return inside }
	warp.SessionID = func() string { return sessionID }
	t.Cleanup(func() {
		warp.Detect = origDetect
		warp.SessionID = origID
	})
}

// execProbe is a MockExecutor whose Exec first runs during(), so a test can
// observe state while the attach is "running".
type execProbe struct {
	sshpkg.MockExecutor
	during func()
}

func (p *execProbe) Exec(cmd string, onDisconnect func()) error {
	if p.during != nil {
		p.during()
	}
	return p.MockExecutor.Exec(cmd, onDisconnect)
}
```

- [ ] **Step 2: Update the four existing execTmuxSession tests to the new signature**

In `cmd/cmd_test.go`, change every call `execTmuxSession(mock, "myhost", "api", false)` to `execTmuxSession(mock, "builder", "myhost", "api")` and `execTmuxSession(mock, "myhost", "out of band", false)` to `execTmuxSession(mock, "builder", "myhost", "out of band")`. There are four: `TestExecTmuxSession_PrintsResumeHintOnCleanExit`, `TestExecTmuxSession_PrintsResumeHintOnceOnDisconnect`, `TestExecTmuxSession_DoesNotPrintHintOnFailure`, `TestExecTmuxSession_InvalidDirectResumeNamePrintsPickerFallback`.

- [ ] **Step 3: Write the failing tests** (append to `cmd/cmd_test.go`; add `"errors"` and `"io/fs"` to the imports)

```go
// --- Warp attach ---

func TestExecTmuxSession_OutsideWarpSendsBareAttach(t *testing.T) {
	useTempConfigDir(t)
	forceWarp(t, false, "")
	mock := &sshpkg.MockExecutor{}

	captureStdout(t, func() { _ = execTmuxSession(mock, "builder-104", "myhost", "api") })

	if got := mock.ExecCalls[0]; got != "tmux new -A -s 'api'" {
		t.Errorf("Exec = %q, want bare attach", got)
	}
}

func TestExecTmuxSession_InsideWarpSendsChainAndTracksLiveRecord(t *testing.T) {
	useTempConfigDir(t)
	const id = "aca3a00ac45440f19afea15ac826acd0"
	forceWarp(t, true, id)
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	t.Setenv("WARP_CLIENT_VERSION", "v1")

	probe := &execProbe{}
	probe.during = func() {
		p, s, err := warp.LoadLive(warp.LiveDir(), id)
		if err != nil || p != "builder-104" || s != "api" {
			t.Errorf("live record during Exec = %q, %q, %v; want builder-104, api", p, s, err)
		}
	}

	captureStdout(t, func() { _ = execTmuxSession(probe, "builder-104", "myhost", "api") })

	want := "tmux new -A -s 'api' \\; set-option set-titles on \\; set-option set-titles-string '#S · #T' \\; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \\; set-environment WARP_CLIENT_VERSION 'v1' \\; set-option -g allow-passthrough on"
	if got := probe.ExecCalls[0]; got != want {
		t.Errorf("Exec =\n%s\nwant\n%s", got, want)
	}
	if _, _, err := warp.LoadLive(warp.LiveDir(), id); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("live record after Exec: err = %v, want cleared", err)
	}
}

func TestExecTmuxSession_InsideWarpClearsLiveRecordOnError(t *testing.T) {
	useTempConfigDir(t)
	const id = "aca3a00ac45440f19afea15ac826acd0"
	forceWarp(t, true, id)
	mock := &sshpkg.MockExecutor{ExecErr: exitWith(t, 127)}

	captureStdout(t, func() { _ = execTmuxSession(mock, "builder-104", "myhost", "api") })

	if _, _, err := warp.LoadLive(warp.LiveDir(), id); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("live record after failed Exec: err = %v, want cleared", err)
	}
}

func TestExecTmuxSession_InsideWarpWithoutSessionIDSkipsRecord(t *testing.T) {
	dir := useTempConfigDir(t)
	forceWarp(t, true, "")
	mock := &sshpkg.MockExecutor{}

	captureStdout(t, func() { _ = execTmuxSession(mock, "builder-104", "myhost", "api") })

	if !strings.HasPrefix(mock.ExecCalls[0], "tmux new -A -s 'api' \\; ") {
		t.Errorf("Exec = %q, want the Warp chain even without a session id", mock.ExecCalls[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "warp", "live")); !os.IsNotExist(err) {
		t.Error("no live dir should be created without a session id")
	}
}

func TestExecTmuxSession_LocalHintDerivedFromProfileName(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}

	out := captureStdout(t, func() { _ = execTmuxSession(mock, localProfileName, "localhost", "work") })

	if want := "To continue this session, run devbox resume --local work\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./cmd/ 2>&1 | head -20`
Expected: build failure (`cannot use ... as bool value` / type mismatch on the fourth argument in calls to `execTmuxSession`; both signatures have four parameters).

- [ ] **Step 5: Change execTmuxSession and its call sites**

In `cmd/root.go`, add `"os"` and `"github.com/hiennguyenle/devbox/internal/warp"` to the imports, then replace the `execTmuxSession` function with:

```go
// execTmuxSession attaches sessionName on the resolved profile and prints
// the resume hint when the session ends or the connection first drops.
//
// Inside Warp it sends the chained attach (title, env, passthrough) and
// keeps a live record for this Warp tab while the attach runs, so a tab
// restored after a reboot can find its session again. The record is
// cleared whenever the attach returns, for any reason; it survives only
// when the process dies without returning.
func execTmuxSession(client sshpkg.Executor, profName, host, sessionName string) error {
	local := profName == localProfileName

	hintShown := false
	showResumeHint := func() {
		if hintShown {
			return
		}
		hintShown = true
		resumeCmd := "devbox resume"
		if local {
			resumeCmd = "devbox resume --local"
		}
		if validate.SessionName(sessionName) != nil {
			fmt.Printf("To continue this session, run %s\n", resumeCmd)
			return
		}
		fmt.Printf("To continue this session, run %s %s\n", resumeCmd, sessionName)
	}

	attach := tmux.NewAttachSession(sessionName)
	if warp.Detect() {
		protocol, clientVersion := warp.Env()
		attach = tmux.WarpAttachSession(sessionName, protocol, clientVersion)
		if id := warp.SessionID(); id != "" {
			liveDir := warp.LiveDir()
			if err := warp.SaveLive(liveDir, id, profName, sessionName); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save warp live record: %v\n", err)
			}
			defer warp.ClearLive(liveDir, id)
		}
	}

	if err := handleExecErr(client.Exec(attach, showResumeHint), host, local); err != nil {
		return err
	}
	showResumeHint()
	return nil
}
```

In `cmd/root.go` `runRoot`, change the last line to:

```go
	return execTmuxSession(client, name, host, defSession)
```

In `cmd/attach.go` `runAttachPicker`, change the two calls:

```go
		return execTmuxSession(client, profName, host, defaultSession)
```

```go
	return execTmuxSession(client, profName, host, final.chosen)
```

Keep the line `local := profName == localProfileName` in `runAttachPicker`; `listSessionsForPicker(client, host, local)` still uses it.

In `cmd/new.go` `runNew`, change the last line to:

```go
	return execTmuxSession(client, profName, host, name)
```

In `cmd/resume.go` `runResume`, change the named-resume line to:

```go
		return execTmuxSession(client, profName, host, name)
```

- [ ] **Step 6: Run the full suite to verify it passes**

Run: `go test ./...`
Expected: all packages `ok`. If `go vet` complains about an unused variable in `attach.go`, re-check Step 5.

- [ ] **Step 7: Commit**

```bash
git add cmd/root.go cmd/attach.go cmd/new.go cmd/resume.go cmd/cmd_test.go
git commit -m "feat(cmd): Warp attach chain and live record in execTmuxSession"
```

---

### Task 7: `devbox warp sync` and `devbox warp hook` (`cmd/warp.go`)

**Files:**
- Create: `cmd/warp.go`
- Modify: `internal/validate/validate.go:16-21` (reserved names)
- Test: `cmd/cmd_test.go`, `internal/validate/validate_test.go`

**Interfaces:**
- Consumes: `warp.TabConfigsDir`, `warp.Sync`, `warp.Change`, `warp.ActionSkipped`, `warp.Hook` (Tasks 1, 2, 4); `config.Exists`, `config.LoadDefault`.
- Produces:
  - `var warpTabConfigsDir = warp.TabConfigsDir` — injectable for tests; Task 9 uses it.
  - `func printWarpChanges(out io.Writer, changes []warp.Change)` — Task 9 uses it.
  - Cobra commands `warpCmd`, `warpSyncCmd`, `warpHookCmd` and handlers `runWarpSync`, `runWarpHook`. Task 8 adds `warpRestoreCmd` to the same file.

- [ ] **Step 1: Write the failing validate test**

In `internal/validate/validate_test.go` `TestProfileName_ReservedRejected`, add `"warp"` to the `reserved` slice:

```go
	reserved := []string{"list", "new", "resume", "kill", "rename", "shell", "version", "profile", "connect", "help", "completion", "local", "warp"}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/validate/ -run TestProfileName_ReservedRejected -v`
Expected: FAIL, `ProfileName("warp") should be rejected`.

- [ ] **Step 3: Reserve the name**

In `internal/validate/validate.go`, change the `reservedNames` literal's last line from `"local": true,` to:

```go
	"local": true, "warp": true,
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/validate/`
Expected: `ok`.

- [ ] **Step 5: Write the failing cmd tests** (append to `cmd/cmd_test.go`)

```go
// --- devbox warp sync / hook ---

func useTempTabConfigsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := warpTabConfigsDir
	warpTabConfigsDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { warpTabConfigsDir = orig })
	return dir
}

func TestWarpSync_WritesFilesAndPrintsHookHint(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)
	tabDir := useTempTabConfigsDir(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpSync(warpSyncCmd, nil) })
	if runErr != nil {
		t.Fatalf("runWarpSync: %v", runErr)
	}

	want := strings.Join([]string{
		"wrote " + filepath.Join(tabDir, "devbox_builder-104.toml"),
		"wrote " + filepath.Join(tabDir, "devbox_local.toml"),
		"",
		"To reattach devbox tabs after a reboot, add to ~/.zshrc:",
		`  eval "$(devbox warp hook zsh)"`,
	}, "\n") + "\n"
	if out != want {
		t.Errorf("stdout =\n%s\nwant\n%s", out, want)
	}
	if data, _ := os.ReadFile(filepath.Join(tabDir, "devbox_builder-104.toml")); !strings.HasPrefix(string(data), warp.Marker+"\n") {
		t.Error("generated file must start with the marker")
	}
}

func TestWarpSync_NoConfigWritesLocalOnly(t *testing.T) {
	useTempConfigDir(t) // no config.yaml saved
	tabDir := useTempTabConfigsDir(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpSync(warpSyncCmd, nil) })
	if runErr != nil {
		t.Fatalf("runWarpSync: %v", runErr)
	}
	if !strings.HasPrefix(out, "wrote "+filepath.Join(tabDir, "devbox_local.toml")+"\n\nTo reattach") {
		t.Errorf("stdout = %q, want only the local file then the hint", out)
	}
}

func TestWarpSync_SkippedLineNamesUnmanagedFile(t *testing.T) {
	useTempConfigDir(t)
	tabDir := useTempTabConfigsDir(t)
	mine := filepath.Join(tabDir, "devbox_local.toml")
	if err := os.WriteFile(mine, []byte("name = \"mine\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { _ = runWarpSync(warpSyncCmd, nil) })

	if !strings.HasPrefix(out, "skipped "+mine+" (not managed by devbox)\n") {
		t.Errorf("stdout = %q, want skipped line first", out)
	}
}

func TestWarpSync_MissingWarpDirErrors(t *testing.T) {
	useTempConfigDir(t)
	orig := warpTabConfigsDir
	warpTabConfigsDir = func() (string, error) {
		return "", fmt.Errorf("warp tab configs directory not found: /nowhere/tab_configs")
	}
	t.Cleanup(func() { warpTabConfigsDir = orig })

	err := runWarpSync(warpSyncCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "/nowhere/tab_configs") {
		t.Errorf("err = %v, want directory-not-found", err)
	}
}

func TestWarpHook_PrintsSnippet(t *testing.T) {
	want, _ := warp.Hook("zsh")
	var runErr error
	out := captureStdout(t, func() { runErr = runWarpHook(warpHookCmd, []string{"zsh"}) })
	if runErr != nil || out != want {
		t.Errorf("runWarpHook: err = %v, out =\n%s\nwant\n%s", runErr, out, want)
	}
}

func TestWarpHook_RejectsUnknownShell(t *testing.T) {
	err := runWarpHook(warpHookCmd, []string{"fish"})
	if err == nil || !strings.Contains(err.Error(), `unsupported shell "fish"`) {
		t.Errorf("err = %v", err)
	}
}
```

Add `"fmt"` to the test file's imports if it is not already there.

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./cmd/ 2>&1 | head -10`
Expected: build failure, `undefined: warpTabConfigsDir`, `undefined: runWarpSync`.

- [ ] **Step 7: Write the implementation**

```go
// cmd/warp.go
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/hiennguyenle/devbox/internal/config"
	"github.com/hiennguyenle/devbox/internal/warp"
	"github.com/spf13/cobra"
)

var warpCmd = &cobra.Command{
	Use:   "warp",
	Short: "Warp terminal integration",
}

var warpSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Generate Warp tab configs for every profile",
	Args:  cobra.NoArgs,
	RunE:  runWarpSync,
}

var warpHookCmd = &cobra.Command{
	Use:   "hook SHELL",
	Short: "Print the shell snippet that reattaches restored Warp tabs (zsh, bash)",
	Args:  cobra.ExactArgs(1),
	RunE:  runWarpHook,
}

func init() {
	warpCmd.AddCommand(warpSyncCmd, warpHookCmd)
	rootCmd.AddCommand(warpCmd)
}

// warpTabConfigsDir is a var so tests can point sync at a temp dir.
var warpTabConfigsDir = warp.TabConfigsDir

func runWarpSync(cmd *cobra.Command, args []string) error {
	dir, err := warpTabConfigsDir()
	if err != nil {
		return err
	}
	var cfg *config.Config
	if config.Exists() {
		cfg, err = config.LoadDefault()
		if err != nil {
			return err
		}
	}
	changes, err := warp.Sync(dir, cfg)
	printWarpChanges(os.Stdout, changes)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("To reattach devbox tabs after a reboot, add to ~/.zshrc:")
	fmt.Println(`  eval "$(devbox warp hook zsh)"`)
	return nil
}

// printWarpChanges renders one line per file: "<action> <path>".
func printWarpChanges(out io.Writer, changes []warp.Change) {
	for _, c := range changes {
		line := c.Action + " " + c.Path
		if c.Action == warp.ActionSkipped {
			line += " (not managed by devbox)"
		}
		fmt.Fprintln(out, line)
	}
}

func runWarpHook(cmd *cobra.Command, args []string) error {
	snippet, err := warp.Hook(args[0])
	if err != nil {
		return err
	}
	fmt.Print(snippet)
	return nil
}
```

- [ ] **Step 8: Run the full suite to verify it passes**

Run: `go test ./...`
Expected: all `ok`.

- [ ] **Step 9: Smoke-check the CLI**

Run: `go run . warp hook zsh && go run . warp hook fish; go run . warp --help`
Expected: the zsh snippet, then `Error: unsupported shell "fish" (zsh, bash)`, then help listing `hook` and `sync`. Do not run `warp sync` here; it writes into the real Warp directory.

- [ ] **Step 10: Commit**

```bash
git add cmd/warp.go cmd/cmd_test.go internal/validate/validate.go internal/validate/validate_test.go
git commit -m "feat(cmd): devbox warp sync and hook; reserve warp as a profile name"
```

---

### Task 8: `devbox warp restore` (`cmd/warp.go`)

**Files:**
- Modify: `cmd/warp.go`
- Test: `cmd/cmd_test.go`

**Interfaces:**
- Consumes: `execTmuxSession(client, profName, host, sessionName)` (Task 6); `warp.Detect`, `warp.SessionID`, `warp.LiveDir`, `warp.LoadLive`, `warp.ClearLive`, `warp.PruneLive`, `warp.SaveLive` (Tasks 1, 3); `requireConfig`, `newClient`, `newLocalClient`, `localProfileName`, `recordProfileSession` from `cmd`.
- Produces: `warpRestoreCmd`, `runWarpRestore`. Restore is a silent no-op outside Warp, without a session id, and when `$TMUX` is set (a shell inside the attached tmux is not a restored tab).

- [ ] **Step 1: Write the failing tests** (append to `cmd/cmd_test.go`; add `"time"` to the imports)

```go
// --- devbox warp restore ---

const testWarpID = "aca3a00ac45440f19afea15ac826acd0"

func injectRemoteMock(t *testing.T) *sshpkg.MockExecutor {
	t.Helper()
	mock := &sshpkg.MockExecutor{}
	orig := newClient
	newClient = func(*config.Profile) sshpkg.Executor { return mock }
	t.Cleanup(func() { newClient = orig })
	return mock
}

func injectLocalMock(t *testing.T) *sshpkg.MockExecutor {
	t.Helper()
	mock := &sshpkg.MockExecutor{}
	orig := newLocalClient
	newLocalClient = func() sshpkg.Executor { return mock }
	t.Cleanup(func() { newLocalClient = orig })
	return mock
}

func TestWarpRestore_OutsideWarpIsSilent(t *testing.T) {
	useTempConfigDir(t)
	forceWarp(t, false, "")
	mock := injectRemoteMock(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })

	if runErr != nil || out != "" || len(mock.ExecCalls) != 0 {
		t.Errorf("err = %v, out = %q, execs = %v; want nothing", runErr, out, mock.ExecCalls)
	}
}

func TestWarpRestore_NoRecordIsSilent(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)
	forceWarp(t, true, testWarpID)
	mock := injectRemoteMock(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })

	if runErr != nil || out != "" || len(mock.ExecCalls) != 0 {
		t.Errorf("err = %v, out = %q, execs = %v; want nothing", runErr, out, mock.ExecCalls)
	}
}

func TestWarpRestore_InsideTmuxIsSilentAndKeepsRecord(t *testing.T) {
	useTempConfigDir(t)
	forceWarp(t, true, testWarpID)
	if err := warp.SaveLive(warp.LiveDir(), testWarpID, "builder-104", "api"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX", "/tmp/tmux-501/default,123,0")
	mock := injectRemoteMock(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })

	if runErr != nil || out != "" || len(mock.ExecCalls) != 0 {
		t.Errorf("err = %v, out = %q, execs = %v; want nothing inside tmux", runErr, out, mock.ExecCalls)
	}
	if _, _, err := warp.LoadLive(warp.LiveDir(), testWarpID); err != nil {
		t.Errorf("the outer tab's record must survive a nested shell, err = %v", err)
	}
}

func TestWarpRestore_ReattachesRecordedSession(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttytest")
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)
	forceWarp(t, true, testWarpID)
	if err := warp.SaveLive(warp.LiveDir(), testWarpID, "builder-104", "api"); err != nil {
		t.Fatal(err)
	}
	mock := injectRemoteMock(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })
	if runErr != nil {
		t.Fatalf("runWarpRestore: %v", runErr)
	}

	if !strings.HasPrefix(out, "Restoring builder-104:api\n") {
		t.Errorf("stdout = %q, want Restoring line first", out)
	}
	if len(mock.ExecCalls) != 1 || !strings.HasPrefix(mock.ExecCalls[0], "tmux new -A -s 'api' \\; ") {
		t.Errorf("Exec calls = %v, want one Warp attach of api", mock.ExecCalls)
	}
	if got, _ := config.GetSession(config.SessionsDir(), "builder-104", "ttytest"); got != "api" {
		t.Errorf("terminal affinity = %q, want api", got)
	}
	if _, _, err := warp.LoadLive(warp.LiveDir(), testWarpID); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("live record after restore: err = %v, want cleared", err)
	}
}

func TestWarpRestore_LocalRecordUsesLocalExecutor(t *testing.T) {
	useTempConfigDir(t) // no config needed for local
	forceWarp(t, true, testWarpID)
	if err := warp.SaveLive(warp.LiveDir(), testWarpID, localProfileName, "work"); err != nil {
		t.Fatal(err)
	}
	remote := injectRemoteMock(t)
	local := injectLocalMock(t)

	out := captureStdout(t, func() { _ = runWarpRestore(warpRestoreCmd, nil) })

	if !strings.HasPrefix(out, "Restoring local:work\n") {
		t.Errorf("stdout = %q", out)
	}
	if len(local.ExecCalls) != 1 || len(remote.ExecCalls) != 0 {
		t.Errorf("local execs = %v, remote execs = %v; want one local attach", local.ExecCalls, remote.ExecCalls)
	}
}

func TestWarpRestore_DeletedProfileRemovesRecordAndWarns(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)
	forceWarp(t, true, testWarpID)
	if err := warp.SaveLive(warp.LiveDir(), testWarpID, "gone", "api"); err != nil {
		t.Fatal(err)
	}
	mock := injectRemoteMock(t)

	var runErr error
	out := captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })

	if runErr != nil || out != "" || len(mock.ExecCalls) != 0 {
		t.Errorf("err = %v, out = %q, execs = %v; want silent stdout and no attach", runErr, out, mock.ExecCalls)
	}
	if _, _, err := warp.LoadLive(warp.LiveDir(), testWarpID); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("record for a deleted profile should be removed, err = %v", err)
	}
}

func TestWarpRestore_MalformedRecordIsRemoved(t *testing.T) {
	useTempConfigDir(t)
	forceWarp(t, true, testWarpID)
	if err := os.MkdirAll(warp.LiveDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(warp.LiveDir(), testWarpID), []byte("junk\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mock := injectRemoteMock(t)

	var runErr error
	captureStdout(t, func() { runErr = runWarpRestore(warpRestoreCmd, nil) })

	if runErr != nil || len(mock.ExecCalls) != 0 {
		t.Errorf("err = %v, execs = %v; want exit 0 and no attach", runErr, mock.ExecCalls)
	}
	if _, err := os.Stat(filepath.Join(warp.LiveDir(), testWarpID)); !os.IsNotExist(err) {
		t.Error("malformed record should be removed")
	}
}

func TestWarpRestore_PrunesOldRecordsButStillRestoresCurrent(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)
	forceWarp(t, true, testWarpID)
	dir := warp.LiveDir()
	for _, id := range []string{testWarpID, "0000000000000000000000000000dead"} {
		if err := warp.SaveLive(dir, id, "builder-104", "api"); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-8 * 24 * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, id), past, past); err != nil {
			t.Fatal(err)
		}
	}
	mock := injectRemoteMock(t)

	captureStdout(t, func() { _ = runWarpRestore(warpRestoreCmd, nil) })

	if len(mock.ExecCalls) != 1 {
		t.Errorf("a week-old record for this very tab must still restore; execs = %v", mock.ExecCalls)
	}
	if _, err := os.Stat(filepath.Join(dir, "0000000000000000000000000000dead")); !os.IsNotExist(err) {
		t.Error("stale record for another tab should be pruned")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ 2>&1 | head -10`
Expected: build failure, `undefined: runWarpRestore`, `undefined: warpRestoreCmd`.

- [ ] **Step 3: Write the implementation** (in `cmd/warp.go`)

Add to the imports: `"errors"`, `"io/fs"`, `"time"`, and `sshpkg "github.com/hiennguyenle/devbox/internal/ssh"`.

Add the command and register it (change the `init` to `warpCmd.AddCommand(warpSyncCmd, warpHookCmd, warpRestoreCmd)`):

```go
var warpRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Reattach this Warp tab's session after a restart (run by the shell hook)",
	Args:  cobra.NoArgs,
	RunE:  runWarpRestore,
}

// liveRecordMaxAge is how long a live record may sit unused before it is
// treated as belonging to a tab that will never come back.
const liveRecordMaxAge = 7 * 24 * time.Hour

// runWarpRestore is what the shell hook runs at every interactive shell
// start inside Warp. It is silent unless this exact Warp tab was attached
// through devbox when the process died, in which case it reattaches.
func runWarpRestore(cmd *cobra.Command, args []string) error {
	// A shell inside the tmux session devbox just attached is not a restored
	// tab. tmux >= 3.3 rewrites TERM_PROGRAM so the hook never fires there;
	// the TMUX check covers older servers, where a nested restore would fail
	// the attach and then clear the outer tab's record.
	if !warp.Detect() || os.Getenv("TMUX") != "" {
		return nil
	}
	id := warp.SessionID()
	if id == "" {
		return nil
	}
	dir := warp.LiveDir()

	profName, session, err := warp.LoadLive(dir, id)
	// Prune after loading so a week-old record for this very tab still
	// restores; execTmuxSession re-saves it fresh while the attach runs.
	warp.PruneLive(dir, liveRecordMaxAge)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		warp.ClearLive(dir, id)
		fmt.Fprintf(os.Stderr, "warning: removed unreadable warp live record: %v\n", err)
		return nil
	}

	var host string
	var client sshpkg.Executor
	if profName == localProfileName {
		host, client = "localhost", newLocalClient()
	} else {
		cfg, err := requireConfig()
		if err != nil {
			warp.ClearLive(dir, id)
			fmt.Fprintf(os.Stderr, "warning: %v; removed warp live record\n", err)
			return nil
		}
		prof, ok := cfg.Profile(profName)
		if !ok {
			warp.ClearLive(dir, id)
			fmt.Fprintf(os.Stderr, "warning: profile %q no longer exists; removed warp live record\n", profName)
			return nil
		}
		host, client = prof.Host, newClient(prof)
	}

	fmt.Printf("Restoring %s:%s\n", profName, session)
	recordProfileSession(profName, session)
	return execTmuxSession(client, profName, host, session)
}
```

- [ ] **Step 4: Run the full suite to verify it passes**

Run: `go test ./...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add cmd/warp.go cmd/cmd_test.go
git commit -m "feat(cmd): devbox warp restore reattaches a restored Warp tab"
```

---

### Task 9: Auto-sync after `devbox profile` (`cmd/profile.go`)

**Files:**
- Modify: `cmd/profile.go` (`addOrUpdateProfile`, after the "saved" print)
- Test: `cmd/cmd_test.go`

**Interfaces:**
- Consumes: `warp.Detect`, `warp.Sync` (Tasks 1, 2); `warpTabConfigsDir`, `printWarpChanges` (Task 7).
- Produces: `func syncWarpAfterSave(cfg *config.Config)`.

- [ ] **Step 1: Write the failing tests** (append to `cmd/cmd_test.go`)

```go
// --- profile auto-sync ---

func TestSyncWarpAfterSave_OutsideWarpDoesNothing(t *testing.T) {
	forceWarp(t, false, "")
	tabDir := useTempTabConfigsDir(t)
	cfg := &config.Config{DefaultProfile: "b", Profiles: map[string]*config.Profile{"b": {Host: "h", DefaultSession: "work"}}}

	out := captureStdout(t, func() { syncWarpAfterSave(cfg) })

	if out != "" {
		t.Errorf("stdout = %q, want nothing outside Warp", out)
	}
	if entries, _ := os.ReadDir(tabDir); len(entries) != 0 {
		t.Errorf("no files should be written outside Warp, got %d", len(entries))
	}
}

func TestSyncWarpAfterSave_InsideWarpWritesConfigs(t *testing.T) {
	forceWarp(t, true, "")
	tabDir := useTempTabConfigsDir(t)
	cfg := &config.Config{DefaultProfile: "b", Profiles: map[string]*config.Profile{"b": {Host: "h", DefaultSession: "work"}}}

	out := captureStdout(t, func() { syncWarpAfterSave(cfg) })

	want := "wrote " + filepath.Join(tabDir, "devbox_b.toml") + "\n" +
		"wrote " + filepath.Join(tabDir, "devbox_local.toml") + "\n"
	if out != want {
		t.Errorf("stdout =\n%s\nwant\n%s", out, want)
	}
}

func TestSyncWarpAfterSave_MissingDirIsWarningOnly(t *testing.T) {
	forceWarp(t, true, "")
	orig := warpTabConfigsDir
	warpTabConfigsDir = func() (string, error) { return "", fmt.Errorf("warp tab configs directory not found: /nowhere") }
	t.Cleanup(func() { warpTabConfigsDir = orig })
	cfg := &config.Config{Profiles: map[string]*config.Profile{"b": {Host: "h", DefaultSession: "work"}}}

	out := captureStdout(t, func() { syncWarpAfterSave(cfg) }) // must not panic or exit

	if out != "" {
		t.Errorf("stdout = %q, want nothing (warning goes to stderr)", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ 2>&1 | head -5`
Expected: build failure, `undefined: syncWarpAfterSave`.

- [ ] **Step 3: Write the implementation**

In `cmd/profile.go`, add `"github.com/hiennguyenle/devbox/internal/warp"` to the imports. In `addOrUpdateProfile`, replace the final two lines

```go
	fmt.Printf("Profile %q saved to %s\n", name, config.Path())
	return nil
```

with

```go
	fmt.Printf("Profile %q saved to %s\n", name, config.Path())
	syncWarpAfterSave(cfg)
	return nil
```

and append to the file:

```go
// syncWarpAfterSave regenerates the Warp tab configs after a profile change
// when devbox is running inside Warp, so the + menu matches config.yaml
// without a separate `devbox warp sync`. Best-effort: any failure is a
// warning, never a failed profile command.
func syncWarpAfterSave(cfg *config.Config) {
	if !warp.Detect() {
		return
	}
	dir, err := warpTabConfigsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: warp sync: %v\n", err)
		return
	}
	changes, err := warp.Sync(dir, cfg)
	printWarpChanges(os.Stdout, changes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: warp sync: %v\n", err)
	}
}
```

- [ ] **Step 4: Run the full suite to verify it passes**

Run: `go test ./...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add cmd/profile.go cmd/cmd_test.go
git commit -m "feat(cmd): regenerate Warp tab configs after profile save inside Warp"
```

---

### Task 10: README

**Files:**
- Modify: `README.md` (Usage list; new section before `## Requirements`)

- [ ] **Step 1: Add the warp commands to the Usage block**

In the `## Usage` code block, after the `devbox shell` line, insert:

```text
devbox warp sync        # generate Warp tab configs (one per profile + local)
devbox warp hook zsh    # print the rc snippet that reattaches restored Warp tabs
devbox warp restore     # reattach this Warp tab's session (run by the hook)
```

- [ ] **Step 2: Add the Warp section**

Insert before `## Requirements`:

```markdown
## Warp

Inside the [Warp](https://warp.dev) terminal, devbox does three extra things.
Nothing changes in other terminals.

**Tabs in the `+` menu.** `devbox warp sync` writes one tab config per
profile, plus one for local mode, into `~/.warp/tab_configs/` (Linux:
`~/.local/share/warp-terminal/tab_configs/`). Warp's `+` menu then lists
"devbox: builder-104" and friends; picking one asks for a session name
(default: the profile's default session) and opens a colored tab running
`devbox resume`. Files devbox wrote start with a marker comment and are the
only ones it ever overwrites or removes. `devbox profile` re-runs the sync
after saving when it runs inside Warp.

**Live tab title.** While attached, the Warp tab reads
`<session> · <pane title>`. Claude Code and Codex write their current task
into the pane title, so the tab tracks what the agent is doing. This is set
on the attached tmux session only; other terminals keep your tmux.conf.

**Reattach after a reboot.** Add the hook once:

```sh
eval "$(devbox warp hook zsh)"     # ~/.zshrc; use "hook bash" for ~/.bashrc
```

When Warp restores its tabs after a reboot, a Warp quit, or a crash, each tab
that was attached through devbox reattaches its own session. Tabs you had
detached or closed do nothing. Set `DEVBOX_WARP_RESTORE=0` to turn it off.

**Notifications from remote Claude Code.** devbox forwards Warp's
`WARP_CLI_AGENT_PROTOCOL_VERSION` and `WARP_CLIENT_VERSION` into the tmux
session and turns on `allow-passthrough` on the running server (in memory
only). For Warp's Claude Code plugin to reach Warp through tmux, all of
these must hold:

- tmux 3.3 or newer on the remote;
- Claude Code started in a tmux window or pane opened *after* devbox
  attached (older shells never see the variables);
- a version of the Warp plugin that wraps its notification for tmux. No
  released version does this yet; until then the plumbing is inert.
```

- [ ] **Step 3: Check the README renders and the suite is green**

Run: `go test ./... && grep -n '^## ' README.md`
Expected: all `ok`; the section list shows `## Warp` between `## Session Tracking` and `## Requirements`.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: Warp integration usage"
```

---

## Done criteria

- `go test ./...` green after every task.
- `go vet ./...` clean.
- `devbox warp sync` on the developer's machine lists a `wrote` line per profile plus local and prints the hook hint; the files appear in Warp's `+` menu.
- Opening one from the `+` menu, accepting the default session, produces a tab attached through devbox whose title reads `<session> · <pane title>`.
- With the hook in `~/.zshrc`, quitting Warp while attached and reopening it reattaches the same session in the same tab and prints `Restoring <profile>:<session>`.
