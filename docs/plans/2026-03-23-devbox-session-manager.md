# Devbox Session Manager Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a small local CLI named `devbox` that connects to `builder-104` and manages remote `tmux` sessions with commands like default attach, list, resume, create, kill, rename, help, and raw shell access.

**Architecture:** Implement `devbox` as a standalone Bash script in this repo so it can grow beyond a one-off shell alias. The script stays local, uses `ssh` as the transport, treats remote `tmux` sessions on `builder-104` as the single source of truth, and exposes a thin command layer over a small set of remote `tmux` commands. Tests should mock `ssh` so command parsing and remote command generation are verified without touching the real host.

**Tech Stack:** Bash, `ssh`, remote `tmux`, local shell test script, Markdown docs

---

## Constraints And Product Decisions

- Target host is fixed to `builder-104` for v1.
- Default session name is `work`.
- `devbox` should support these actions only in v1:
  - `devbox`
  - `devbox --list`
  - `devbox --resume NAME`
  - `devbox --new NAME`
  - `devbox --kill NAME`
  - `devbox --rename OLD NEW`
  - `devbox --shell`
  - `devbox --help`
- `--resume NAME` and `--new NAME` may intentionally share the same backend behavior: `tmux new -A -s NAME`.
- Do not auto-run `claude` in v1.
- Do not add fuzzy picker, recent-session tracking, or config files in v1.
- Fail fast on invalid arguments with readable help text.

## Repository Layout To Create

- Create: `bin/devbox`
- Create: `tests/devbox_test.sh`
- Create: `README.md`
- Create: `docs/plans/2026-03-23-devbox-session-manager.md`

### Task 1: Scaffold The CLI Script

**Files:**
- Create: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add a test that runs `bin/devbox --help` and expects:
- exit code `0`
- output contains `builder-104`
- output contains each supported flag

Suggested test shape:

```bash
run_devbox --help
assert_status 0
assert_contains "$output" "builder-104"
assert_contains "$output" "--list"
assert_contains "$output" "--resume NAME"
```

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because `bin/devbox` does not exist yet

**Step 3: Write minimal implementation**

Create `bin/devbox` with:
- shebang `#!/usr/bin/env bash`
- `set -euo pipefail`
- constants for `HOST="builder-104"` and `DEFAULT_SESSION="work"`
- `print_help()` function
- `main()` function that recognizes `--help`

Initial help output should document all planned commands, even before they are implemented.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS for the help test

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh README.md docs/plans/2026-03-23-devbox-session-manager.md
git commit -m "feat: scaffold devbox session manager"
```

### Task 2: Add A Simple Test Harness With Mocked SSH

**Files:**
- Modify: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add a test that runs `devbox --list` with a mocked `ssh` executable earlier in `PATH` and expects the script to call remote `tmux list-sessions` without `-t`.

Suggested test shape:

```bash
run_devbox --list
assert_status 0
assert_contains "$mock_log" "ssh builder-104 tmux list-sessions"
```

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because list routing is not implemented

**Step 3: Write minimal implementation**

In `tests/devbox_test.sh`, create helpers:
- temporary directory setup
- fake `ssh` binary that writes argv into a log file
- `run_devbox()` wrapper that injects fake `PATH`
- basic assertion helpers: `assert_status`, `assert_contains`

Keep the fake `ssh` flexible so later tests can inspect commands for interactive and non-interactive modes.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS for harness setup and list test

**Step 5: Commit**

```bash
git add tests/devbox_test.sh bin/devbox
git commit -m "test: add devbox ssh mock harness"
```

### Task 3: Implement Default Attach Behavior

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add a test for running `devbox` with no args. Expect:
- `ssh -t builder-104 "tmux new -A -s work"`
- exit code `0`

Suggested test shape:

```bash
run_devbox
assert_status 0
assert_contains "$mock_log" "ssh -t builder-104 tmux new -A -s work"
```

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because default attach is not implemented

**Step 3: Write minimal implementation**

Add helper functions:
- `run_ssh()` for non-interactive commands
- `run_ssh_tty()` for interactive commands
- `attach_session(session_name)` that uses `tmux new -A -s "$session_name"`

Default `main()` behavior with no args should call `attach_session "$DEFAULT_SESSION"`.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: attach default devbox session"
```

### Task 4: Implement `--list` And Output Formatting

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add tests for `devbox --list` that expect:
- non-interactive ssh usage
- remote command `tmux list-sessions`
- clean behavior when remote tmux returns no sessions

Mock outputs for both cases:
- one or more sessions
- `no server running on ...`

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because output handling is incomplete

**Step 3: Write minimal implementation**

Implement `list_sessions()` that:
- runs `tmux list-sessions`
- if tmux exits non-zero with “no server running”, prints a friendly message like `No active devbox sessions on builder-104.` and exits `0`
- otherwise forwards session output

Do not parse or transform session lines in v1 beyond the empty-state message.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: list remote tmux sessions"
```

### Task 5: Implement `--resume NAME` And `--new NAME`

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add tests that verify:
- `devbox --resume api` calls `ssh -t builder-104 tmux new -A -s api`
- `devbox --new web` calls `ssh -t builder-104 tmux new -A -s web`
- missing names cause exit code `1` and usage output

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because named session actions are not implemented

**Step 3: Write minimal implementation**

In `main()`, add argument parsing for:
- `--resume NAME`
- `--new NAME`

Both should validate that `NAME` is present and non-empty, then call `attach_session "$NAME"`.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: support named devbox sessions"
```

### Task 6: Implement `--kill NAME`

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add tests that verify:
- `devbox --kill api` calls `ssh builder-104 tmux kill-session -t api`
- missing name fails with exit code `1`

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because kill action is not implemented

**Step 3: Write minimal implementation**

Implement `kill_session(session_name)` using non-interactive ssh.

Avoid confirmation prompts in v1.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: add session kill command"
```

### Task 7: Implement `--rename OLD NEW`

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add tests that verify:
- `devbox --rename api backend` calls `ssh builder-104 tmux rename-session -t api backend`
- missing either name fails with exit code `1`

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because rename action is not implemented

**Step 3: Write minimal implementation**

Implement `rename_session(old_name, new_name)` using non-interactive ssh.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: add session rename command"
```

### Task 8: Implement `--shell`

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add a test that verifies:
- `devbox --shell` calls `ssh -t builder-104`
- no remote tmux command is appended

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because shell mode is not implemented

**Step 3: Write minimal implementation**

Implement `open_shell()` that invokes interactive ssh to `builder-104` with no remote command.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "feat: add raw shell mode"
```

### Task 9: Harden Argument Parsing And Errors

**Files:**
- Modify: `bin/devbox`
- Test: `tests/devbox_test.sh`

**Step 1: Write the failing test**

Add tests for:
- unknown flag like `--wat`
- extra unexpected positional args
- empty string names

Each should expect:
- exit code `1`
- error message on stderr
- compact usage hint

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because validation is incomplete

**Step 3: Write minimal implementation**

Add helpers:
- `die()`
- `require_arg()`

Keep parsing simple with a `case` statement and strict arg-count checks.

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh
git commit -m "fix: validate devbox arguments"
```

### Task 10: Write README And Install Instructions

**Files:**
- Create: `README.md`

**Step 1: Write the failing test**

Add a lightweight documentation check in `tests/devbox_test.sh` that expects `README.md` to contain:
- install section
- command summary
- `builder-104`
- `tmux`

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: FAIL because README does not exist yet

**Step 3: Write minimal implementation**

Create `README.md` with:
- project purpose
- dependency list: local `ssh`, remote `tmux`
- install options:
  - symlink `bin/devbox` into `~/.local/bin`
  - or add repo `bin/` to `PATH`
- usage examples for every supported command
- note that persistent sessions depend on `tmux`, not SSH reconnecting itself

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add README.md tests/devbox_test.sh
git commit -m "docs: add devbox usage guide"
```

### Task 11: Run End-To-End Manual Verification

**Files:**
- Verify: `bin/devbox`
- Verify: `README.md`

**Step 1: Write the failing test**

No new automated test. Create a manual verification checklist in `README.md` or a temporary local note:
- `devbox --help`
- `devbox --list`
- `devbox`
- detach from tmux
- reconnect with `devbox --resume work`
- create another session with `devbox --new api`
- list sessions again
- rename session
- kill session
- `devbox --shell`

**Step 2: Run test to verify it fails**

Run: `bash tests/devbox_test.sh`
Expected: PASS on automated tests only; manual checklist still unverified

**Step 3: Write minimal implementation**

No code change. Perform manual verification against the real `builder-104` host once the script is on your `PATH`.

Suggested commands:

```bash
chmod +x bin/devbox
ln -sf "/Users/hien/Repositories/devbox/bin/devbox" "$HOME/.local/bin/devbox"
devbox --help
devbox --list
devbox
devbox --new api
devbox --list
devbox --rename api backend
devbox --kill backend
devbox --shell
```

Expected outcomes:
- interactive actions attach cleanly
- list reflects remote tmux state
- shell mode opens plain SSH session
- reconnecting after disconnect returns to the same tmux session

**Step 4: Run test to verify it passes**

Run: `bash tests/devbox_test.sh`
Expected: PASS

Then complete manual verification checklist.

**Step 5: Commit**

```bash
git add bin/devbox tests/devbox_test.sh README.md
git commit -m "chore: verify devbox workflow"
```

## Notes For Implementation

- Prefer Bash over zsh-specific features inside `bin/devbox` so the script is portable.
- Quote every session name passed to remote commands.
- Keep remote commands as separate argv items when invoking `ssh` to avoid accidental shell-escaping bugs.
- Keep v1 intentionally small; do not add config files, color output, interactive pickers, or auto-launch logic.
- If you later need per-host support, add it in a separate plan instead of leaking it into this one.

## Testing Strategy

- Primary automated tests: `bash tests/devbox_test.sh`
- Manual real-host verification: `builder-104` with remote `tmux` installed
- Regression focus:
  - correct SSH flags for interactive vs non-interactive actions
  - argument validation
  - correct remote `tmux` command selection
  - friendly empty-state handling for `--list`

## Definition Of Done

- `bin/devbox` exists, is executable, and supports the agreed v1 command set.
- `tests/devbox_test.sh` passes locally.
- `README.md` explains install, usage, and tmux-backed persistence.
- Manual verification on `builder-104` confirms sessions survive disconnects and can be resumed.
