# Local Mode Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let devbox manage tmux sessions on the local machine via a `--local` flag (`devbox list --local`, `devbox resume --local`, ...) alongside the existing remote/SSH behavior.

**Architecture:** A new `internal/local.Executor` implements the existing `ssh.Executor` interface (`Run`/`Exec`) by shelling out to `sh -c` instead of `ssh`. A `resolveExecutor` helper in `cmd` picks the transport: `--local` (or positional `local` at the root) uses the built-in reserved `local` profile with no config required; otherwise the existing profile resolution runs. Commands already program against the interface, so they mostly swap their 3-line config/resolve/client preamble for one call. Spec: `docs/superpowers/specs/2026-09-03-local-mode-design.md`.

**Tech Stack:** Go 1.24, cobra, existing `internal/{ssh,tmux,config,validate}` packages, stdlib `os/exec`. No new dependencies.

**Notes for the implementer:**
- Run tests with `go test ./...` from the repo root. Focused runs: `go test ./cmd/ -run '<pattern>' -v`.
- Tests inject executors through package vars (`newClient`, and the new `newLocalClient`) and use `config.SetDirOverride` via `useTempConfigDir(t)`. Global flags (`localFlag`, `profileFlag`) are plain vars; tests that set them must reset them in `t.Cleanup`.
- Commit after each task. Commit style matches `git log` (`feat:`, `fix:`, `docs:`).
- Tasks 1–3 are Phase 1 (the user-facing `devbox list --local` works at the end of Task 3). Tasks 4–5 are Phase 2.

---

### Task 1: local executor

**Files:**
- Create: `internal/local/local.go`
- Test: `internal/local/local_test.go`

**Step 1: Write the failing test**

```go
package local

import (
	"errors"
	"os/exec"
	"testing"
)

func TestRunCapturesOutput(t *testing.T) {
	out, err := (&Executor{}).Run("echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hi" {
		t.Errorf("out = %q, want %q", out, "hi")
	}
}

func TestRunExitError(t *testing.T) {
	_, err := (&Executor{}).Run("exit 3")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("want exit code 3, got %v", err)
	}
}

func TestExecExitCodes(t *testing.T) {
	if err := (&Executor{}).Exec("true", nil); err != nil {
		t.Errorf("Exec true: %v", err)
	}
	if err := (&Executor{}).Exec("false", nil); err == nil {
		t.Error("Exec false: want error, got nil")
	}
}

func TestExecEmptyUsesShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/true")
	if err := (&Executor{}).Exec("", nil); err != nil {
		t.Errorf("Exec empty: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/local/ -v`
Expected: build failure — package `local` does not exist.

**Step 3: Write minimal implementation**

```go
// Package local runs devbox's command strings on this machine instead of a
// remote host. Executor implements ssh.Executor so commands stay
// transport-agnostic.
package local

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Executor struct {
	Verbose bool
}

// Run executes a command through sh and captures its output, mirroring
// ssh.Client.Run.
func (e *Executor) Run(command string) (string, error) {
	if e.Verbose {
		fmt.Fprintf(os.Stderr, "local %s\n", command)
	}
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Exec runs a command with the terminal attached. An empty command opens the
// user's shell, mirroring `devbox shell`. onDisconnect is accepted for
// interface parity and never called: there is no transport to lose.
func (e *Executor) Exec(command string, onDisconnect func()) error {
	var cmd *exec.Cmd
	if command == "" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = exec.Command(shell)
		if e.Verbose {
			fmt.Fprintf(os.Stderr, "local %s\n", shell)
		}
	} else {
		cmd = exec.Command("sh", "-c", command)
		if e.Verbose {
			fmt.Fprintf(os.Stderr, "local %s\n", command)
		}
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/local/ -v`
Expected: PASS (4 tests).

**Step 5: Commit**

```bash
git add internal/local
git commit -m "feat(local): executor that runs devbox commands on this machine"
```

---

### Task 2: reserve the "local" profile name

**Files:**
- Modify: `internal/validate/validate.go` (`reservedNames` map, ~line 16)
- Test: `internal/validate/validate_test.go`

**Step 1: Write the failing test**

Add to the existing profile-name tests in `internal/validate/validate_test.go`:

```go
func TestProfileNameLocalReserved(t *testing.T) {
	if err := ProfileName("local"); err == nil {
		t.Error("profile name \"local\" should be reserved")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/validate/ -run TestProfileNameLocalReserved -v`
Expected: FAIL — no error returned for "local".

**Step 3: Write minimal implementation**

In `reservedNames` (internal/validate/validate.go), add the entry:

```go
var reservedNames = map[string]bool{
	"list": true, "new": true, "resume": true, "kill": true,
	"rename": true, "shell": true, "version": true, "profile": true,
	"connect": true, "help": true, "completion": true,
	"local": true,
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/validate/ -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/validate
git commit -m "feat(validate): reserve profile name local for the built-in profile"
```

---

### Task 3: --local plumbing and `devbox list --local` (Phase 1 done here)

This task changes three shared signatures. Every existing call site and test
call site must be updated in the same commit:
- `handleSSHError(output, err, host)` → `handleSSHError(output, err, host string, local bool)` (call sites: `cmd/list.go`, `cmd/kill.go`, `cmd/rename.go`, `cmd/attach.go:listSessionsForPicker`, tests at `cmd/cmd_test.go` ~114/121/128)
- `handleExecErr(err, host)` → `handleExecErr(err, host string, local bool)` (call site: `execTmuxSession`, tests ~134/141/145)
- `execTmuxSession(client, host, sessionName)` → `execTmuxSession(client, host, sessionName string, local bool)` (call sites: `cmd/new.go`, `cmd/resume.go` via `runAttachPicker`, `cmd/root.go`, tests ~155/174/190/206)

Pass `false` for the new bool at every *existing* (remote) call site so remote
behavior is byte-for-byte unchanged; this task only flips `list.go` to the new
preamble.

**Files:**
- Modify: `cmd/root.go`, `cmd/list.go`, `cmd/attach.go` (only `listSessionsForPicker` for now), `cmd/kill.go` + `cmd/rename.go` + `cmd/new.go` + `cmd/resume.go` (only the signature pass-throughs), `cmd/cmd_test.go`

**Step 1: Write the failing tests**

Add to `cmd/cmd_test.go`:

```go
// withLocal enables --local and swaps the local executor for mock. No config
// file is needed, matching real --local behavior.
func withLocal(t *testing.T, mock *sshpkg.MockExecutor) {
	t.Helper()
	orig := newLocalClient
	newLocalClient = func() sshpkg.Executor { return mock }
	t.Cleanup(func() { newLocalClient = orig })
	localFlag = true
	t.Cleanup(func() { localFlag = false })
}

func TestListLocalNoConfig(t *testing.T) {
	useTempConfigDir(t) // empty dir: no config file exists
	mock := &sshpkg.MockExecutor{RunOutput: "work: 1 windows"}
	withLocal(t, mock)

	out := captureStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})
	if !strings.Contains(out, "work: 1 windows") {
		t.Errorf("output = %q", out)
	}
	if len(mock.RunCalls) != 1 || !strings.Contains(mock.RunCalls[0], "tmux list-sessions") {
		t.Errorf("RunCalls = %v", mock.RunCalls)
	}
}

func TestListLocalNoSessions(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{RunOutput: "no server running on /tmp/tmux-501/default", RunErr: exitWith(t, 1)}
	withLocal(t, mock)

	out := captureStdout(t, func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList: %v", err)
		}
	})
	if !strings.Contains(out, "No active sessions on localhost.") {
		t.Errorf("output = %q", out)
	}
}

func TestLocalConflictsWithProfileFlag(t *testing.T) {
	useTempConfigDir(t)
	withLocal(t, &sshpkg.MockExecutor{})
	profileFlag = "builder-104"
	t.Cleanup(func() { profileFlag = "" })

	err := runList(listCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("want conflict error, got %v", err)
	}
}

func TestHandleSSHErrorLocal(t *testing.T) {
	err := handleSSHError("sh: tmux: command not found", exitWith(t, 127), "localhost", true)
	if err == nil || !strings.Contains(err.Error(), "tmux is not installed locally") {
		t.Errorf("got %v", err)
	}
	if err := handleSSHError("", exitWith(t, 1), "localhost", true); err == nil ||
		!strings.Contains(err.Error(), "local command failed") {
		t.Errorf("got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run 'TestListLocal|TestLocalConflicts|TestHandleSSHErrorLocal' -v`
Expected: build failure — `newLocalClient`, `localFlag` undefined.

**Step 3: Implement**

In `cmd/root.go`:

1. Add `localFlag` to the flag vars and register it in `init()`:

```go
var (
	verbose     bool
	profileFlag string
	localFlag   bool
	version     = "dev"
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
	rootCmd.PersistentFlags().StringVarP(&profileFlag, "profile", "p", "", "profile to act on (overrides last-used/default)")
	rootCmd.PersistentFlags().BoolVar(&localFlag, "local", false, "act on tmux sessions on this machine instead of a remote profile")
}
```

2. Add the import `"github.com/hiennguyenle/devbox/internal/local"` and, next to `newClient`:

```go
// localProfileName is the built-in profile for tmux on this machine. It is
// reserved (see internal/validate) and never appears in config.yaml.
const localProfileName = "local"

// localDefaultSession is what bare `devbox --local` attaches to.
const localDefaultSession = "work"

// newLocalClient is a var so tests can inject a MockExecutor.
var newLocalClient = func() sshpkg.Executor {
	return &local.Executor{Verbose: verbose}
}

// resolveExecutor decides this invocation's transport and returns the acting
// profile name, a host label for messages, the session to attach when none
// is named, and the executor. --local (or positional "local" at the root)
// needs no config; otherwise config is required and resolved normally.
func resolveExecutor(explicit string) (name, host, defSession string, client sshpkg.Executor, err error) {
	if localFlag || explicit == localProfileName {
		if profileFlag != "" || (explicit != "" && explicit != localProfileName) {
			return "", "", "", nil, fmt.Errorf("--local cannot be combined with a profile")
		}
		return localProfileName, "localhost", localDefaultSession, newLocalClient(), nil
	}
	cfg, err := requireConfig()
	if err != nil {
		return "", "", "", nil, err
	}
	name, prof, err := resolveProfile(cfg, explicit)
	if err != nil {
		return "", "", "", nil, err
	}
	return name, prof.Host, prof.DefaultSession, newClient(prof), nil
}
```

3. Update the three shared helpers (remote behavior unchanged via the new bool):

```go
func handleSSHError(output string, err error, host string, local bool) error {
	if err == nil {
		return nil
	}
	if strings.Contains(output, "command not found") {
		if local {
			return fmt.Errorf("tmux is not installed locally")
		}
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	if local {
		return fmt.Errorf("local command failed: %w", err)
	}
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox profile\"", host)
}

func handleExecErr(err error, host string, local bool) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 127 {
		if local {
			return fmt.Errorf("tmux is not installed locally")
		}
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return err
}

func execTmuxSession(client sshpkg.Executor, host, sessionName string, local bool) error {
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

	if err := handleExecErr(client.Exec(tmux.NewAttachSession(sessionName), showResumeHint), host, local); err != nil {
		return err
	}
	showResumeHint()
	return nil
}
```

4. Update `runRoot` to the new preamble (positional `local` now works because
   `resolveExecutor` special-cases it):

```go
func runRoot(cmd *cobra.Command, args []string) error {
	explicit := ""
	if len(args) == 1 {
		explicit = args[0]
	}
	name, host, defSession, client, err := resolveExecutor(explicit)
	if err != nil {
		return err
	}
	recordProfileSession(name, defSession)
	return execTmuxSession(client, host, defSession, name == localProfileName)
}
```

In `cmd/list.go`, replace `runList`'s body:

```go
func runList(cmd *cobra.Command, args []string) error {
	_, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if isNoServerOutput(output) {
			fmt.Printf("No active sessions on %s.\n", host)
			return nil
		}
		return handleSSHError(output, err, host, localFlag)
	}

	fmt.Println(output)
	return nil
}
```

(list.go drops its `resolveProfile`/`requireConfig` use; no import changes
needed.)

In `cmd/attach.go`, give `listSessionsForPicker` the bool and pass it through:

```go
func listSessionsForPicker(client sshpkg.Executor, host string, local bool) ([]sessionItem, error) {
	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if isNoServerOutput(output) {
			return nil, nil
		}
		return nil, handleSSHError(output, err, host, local)
	}
	return parseSessions(output), nil
}
```

Then fix every remaining compile error by passing `false` (remote) at the
existing call sites in `cmd/kill.go`, `cmd/rename.go`, `cmd/new.go`,
`cmd/resume.go` (`runAttachPicker` still calls
`execTmuxSession(client, prof.Host, ..., false)` for now), and the existing
test call sites in `cmd/cmd_test.go` (`handleSSHError(..., "myhost", false)`,
`handleExecErr(..., "myhost", false)`, `execTmuxSession(mock, "myhost", "api", false)`).

**Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS, including the new local tests and all pre-existing tests.

**Step 5: Manual smoke check**

```bash
go build -o /tmp/devbox-local .
/tmp/devbox-local list --local        # prints local tmux sessions (or "No active sessions on localhost.")
/tmp/devbox-local list --local -p x   # errors: --local cannot be combined with a profile
```

**Step 6: Commit**

```bash
git add cmd
git commit -m "feat: --local flag with local executor, list --local support"
```

---

### Task 4: remaining commands (Phase 2)

Switch `new`, `resume`, `kill`, `rename`, `shell` to `resolveExecutor`, and
let `runAttachPicker` work without a `*config.Profile`.

**Files:**
- Modify: `cmd/new.go`, `cmd/resume.go`, `cmd/kill.go`, `cmd/rename.go`, `cmd/shell.go`, `cmd/attach.go` (`runAttachPicker`)
- Test: `cmd/cmd_test.go`

**Step 1: Write the failing tests**

Add to `cmd/cmd_test.go`:

```go
func TestRootLocalAttachesDefaultSession(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	withLocal(t, mock)

	if err := runRoot(rootCmd, nil); err != nil {
		t.Fatalf("runRoot: %v", err)
	}
	if !mock.LastExecContains("tmux new -A -s 'work'") {
		t.Errorf("ExecCalls = %v", mock.ExecCalls)
	}
}

func TestNewLocalResumeHint(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttys001")
	mock := &sshpkg.MockExecutor{}
	withLocal(t, mock)

	out := captureStdout(t, func() {
		if err := runNew(newCmd, []string{"task1"}); err != nil {
			t.Fatalf("runNew: %v", err)
		}
	})
	if !mock.LastExecContains("tmux new -A -s 'task1'") {
		t.Errorf("ExecCalls = %v", mock.ExecCalls)
	}
	if !strings.Contains(out, "devbox resume --local task1") {
		t.Errorf("output = %q", out)
	}
}

func TestResumeLocalNamed(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttys001")
	mock := &sshpkg.MockExecutor{}
	withLocal(t, mock)

	if err := runResume(resumeCmd, []string{"task1"}); err != nil {
		t.Fatalf("runResume: %v", err)
	}
	if !mock.LastExecContains("tmux new -A -s 'task1'") {
		t.Errorf("ExecCalls = %v", mock.ExecCalls)
	}
	if name, err := config.GetSession(config.SessionsDir(), "local", "ttys001"); err != nil || name != "task1" {
		t.Errorf("session record = %q, %v", name, err)
	}
}

func TestKillLocalClearsSessionRecords(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttys001")
	if err := config.SaveSession(config.SessionsDir(), "local", "ttys001", "task1"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	mock := &sshpkg.MockExecutor{}
	withLocal(t, mock)

	out := captureStdout(t, func() {
		if err := runKill(killCmd, []string{"task1"}); err != nil {
			t.Fatalf("runKill: %v", err)
		}
	})
	if !mock.LastRunContains("kill-session") {
		t.Errorf("RunCalls = %v", mock.RunCalls)
	}
	if !strings.Contains(out, "Killed \"task1\"") {
		t.Errorf("output = %q", out)
	}
	if _, err := config.GetSession(config.SessionsDir(), "local", "ttys001"); err == nil {
		t.Error("local session record should be cleared after kill")
	}
}

func TestShellLocal(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	withLocal(t, mock)

	if err := runShell(shellCmd, nil); err != nil {
		t.Fatalf("runShell: %v", err)
	}
	if len(mock.ExecCalls) != 1 || mock.ExecCalls[0] != "" {
		t.Errorf("ExecCalls = %v", mock.ExecCalls)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run 'TestRootLocal|TestNewLocal|TestResumeLocal|TestKillLocal|TestShellLocal' -v`
Expected: FAIL — e.g. `runShell` still requires a config file, `runNew`
resolves a remote profile.

**Step 3: Implement**

`cmd/new.go` — replace the config/resolve/client preamble:

```go
	profName, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	fmt.Printf("Creating session %q...\n", name)
	recordProfileSession(profName, name)

	return execTmuxSession(client, host, name, profName == localProfileName)
```

(Drop the now-unused `requireConfig`/`resolveProfile` calls; imports stay
valid — remove any that become unused.)

`cmd/resume.go`:

```go
func runResume(cmd *cobra.Command, args []string) error {
	profName, host, defSession, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	// Named resume — direct attach
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		recordProfileSession(profName, name)
		return execTmuxSession(client, host, name, profName == localProfileName)
	}

	// No name — interactive picker, pre-select terminal's last session on this profile
	preSelect := ""
	if tty, err := config.ResolveTTY(); err == nil {
		if name, err := config.GetSession(config.SessionsDir(), profName, tty); err == nil {
			preSelect = name
		}
	}

	return runAttachPicker(profName, host, defSession, client, preSelect)
}
```

`cmd/attach.go` — `runAttachPicker` no longer takes `*config.Profile`:

```go
// runAttachPicker shows an interactive session picker for a resolved profile.
// preSelect is the session name to highlight initially (e.g. the terminal's
// last session on this profile), or "" for default.
func runAttachPicker(profName, host, defaultSession string, client sshpkg.Executor, preSelect string) error {
	local := profName == localProfileName
	sessions, err := listSessionsForPicker(client, host, local)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions available. Creating default session...")
		recordProfileSession(profName, defaultSession)
		return execTmuxSession(client, host, defaultSession, local)
	}

	// Pre-select the cursor to the last used session if known
	cursor := 0
	if preSelect != "" {
		for i, s := range sessions {
			if s.name == preSelect {
				cursor = i
				break
			}
		}
	}

	model := pickerModel{sessions: sessions, cursor: cursor}
	p := tea.NewProgram(model)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	final, ok := result.(pickerModel)
	if !ok {
		return fmt.Errorf("unexpected picker state")
	}
	if final.quitting || final.chosen == "" {
		return nil // user cancelled
	}

	recordProfileSession(profName, final.chosen)
	return execTmuxSession(client, host, final.chosen, local)
}
```

(Remove the now-unused `config` import from attach.go if nothing else uses
it.)

`cmd/kill.go` — preamble, the no-sessions message, and the picker model
(every `prof.Host` becomes `host`):

```go
func runKill(cmd *cobra.Command, args []string) error {
	profName, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	// Direct kill by name
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		return killSession(client, profName, host, name)
	}

	// Interactive mode
	sessions, err := listSessionsForPicker(client, host, localFlag)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Printf("No active sessions on %s.\n", host)
		return nil
	}

	model := killPickerModel{client: client, profile: profName, host: host, sessions: sessions}
	...
```

(the rest of `runKill` is unchanged; `killPickerModel` already works through
the interface). In `killSession`, derive locality from the profile it already
receives:

```go
func killSession(client sshpkg.Executor, profile, host, name string) error {
	output, err := client.Run(tmux.KillSession(name))
	if err != nil {
		if output != "" {
			return fmt.Errorf("kill session %q: %s", name, output)
		}
		return handleSSHError(output, err, host, profile == localProfileName)
	}
	config.DeleteSessionsByName(config.SessionsDir(), profile, name)
	fmt.Printf("Killed %q\n", name)
	return nil
}
```

`cmd/rename.go`:

```go
	_, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	output, err := client.Run(tmux.RenameSession(oldName, newName))
	if err != nil {
		if output != "" {
			return fmt.Errorf("rename session: %s", output)
		}
		return handleSSHError(output, err, host, localFlag)
	}
```

`cmd/shell.go`:

```go
func runShell(cmd *cobra.Command, args []string) error {
	_, _, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	err = client.Exec("", nil)
	// In a raw shell the exit status belongs to the user (`exit 1` is not a
	// devbox failure); only surface non-exit errors like a missing ssh binary.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

**Step 5: Manual smoke check (needs tmux installed)**

```bash
go build -o /tmp/devbox-local .
/tmp/devbox-local new smoke --local     # attaches; detach with prefix-d
/tmp/devbox-local list --local          # shows "smoke"
/tmp/devbox-local resume smoke --local  # reattaches
/tmp/devbox-local kill smoke --local    # Killed "smoke"
```

**Step 6: Commit**

```bash
git add cmd
git commit -m "feat: local mode for new, resume, kill, rename, shell and root attach"
```

---

### Task 5: docs and polish

**Files:**
- Modify: `README.md`, `cmd/root.go` (help strings)

**Step 1: Update help strings**

In `cmd/root.go`:
- `rootCmd.Short`: `"Manage remote tmux sessions over SSH"` → `"Manage tmux sessions over SSH or locally"`.
- verbose flag help: `"print SSH commands being executed"` → `"print the commands being executed (ssh or local)"`.

**Step 2: Update README.md**

Add a `## Local mode` section right after the Setup section (exact text to
insert):

````markdown
## Local mode

Every command also works against tmux on this machine with `--local`, which
needs no profile or config file:

```bash
devbox list --local          # local sessions
devbox new [NAME] --local    # create/attach
devbox resume [NAME] --local # attach (picker if no name)
devbox kill [NAME] --local   # kill (picker if no name)
devbox rename OLD NEW --local
devbox shell --local         # open $SHELL, no tmux
devbox --local               # attach to local session "work"
```

`--local` cannot be combined with `-p/--profile`. The name `local` is
reserved and cannot be used as a profile name.
````

Also add a `--local` mention in any existing command-table/usage section if
one lists the commands (keep the table's existing format).

**Step 3: Verify**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet issues.

**Step 4: Commit**

```bash
git add README.md cmd/root.go
git commit -m "docs: local mode usage and help strings"
```

---

## Done means

- `go test ./...` green; `go vet ./...` clean.
- `devbox list --local` works with no `~/.config/devbox` at all.
- All remote behavior unchanged (no config, flags, output, or reconnect behavior differences when `--local` is absent).
- Commits land as five small, reviewable steps.
