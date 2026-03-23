# Devbox CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI named `devbox` that connects to a configurable remote SSH host and manages tmux sessions with per-terminal session affinity.

**Architecture:** Cobra-based CLI that shells out to the system `ssh` binary. Three internal packages handle config (`~/.config/devbox/`), SSH command construction/execution, and tmux command building. An `Executor` interface decouples SSH execution from command logic for testability. Bubbletea powers the interactive session picker.

**Tech Stack:** Go 1.22+, cobra, bubbletea, lipgloss, gopkg.in/yaml.v3, GoReleaser

**Spec:** `docs/superpowers/specs/2026-03-23-devbox-cli-design.md`

---

## Constraints And Product Decisions

- Single SSH host at a time, configured via `devbox connect`.
- Default session name: `work` (stored in config as `default_session`).
- Per-terminal session tracking via TTY path in `~/.config/devbox/sessions/`.
- Session names: alphanumeric, hyphens, underscores, max 50 chars.
- No real SSH calls in tests — all tests use a mock `Executor`.
- Interactive commands use `syscall.Exec` to replace the process with SSH.
- Non-interactive commands capture output via `exec.Command`.

## File Structure

```
devbox/
├── main.go                          # entry point, calls cmd.Execute()
├── cmd/
│   ├── root.go                      # cobra root (bare "devbox" = attach default session)
│   ├── connect.go                   # interactive SSH host setup
│   ├── list.go                      # list remote tmux sessions
│   ├── resume.go                    # attach named session or per-terminal resume
│   ├── new.go                       # create/attach named session
│   ├── kill.go                      # kill session by name
│   ├── rename.go                    # rename session
│   ├── shell.go                     # raw SSH, no tmux
│   ├── last.go                      # resume most recent session (global)
│   └── attach.go                    # interactive bubbletea session picker
├── internal/
│   ├── config/
│   │   ├── config.go                # read/write config.yaml
│   │   ├── config_test.go           # config tests
│   │   ├── session.go               # per-terminal session tracking
│   │   └── session_test.go          # session tracking tests
│   ├── ssh/
│   │   ├── executor.go              # Executor interface
│   │   ├── ssh.go                   # real Executor implementation
│   │   ├── ssh_test.go              # arg-building tests
│   │   └── mock.go                  # mock Executor for tests
│   ├── tmux/
│   │   ├── tmux.go                  # tmux command builders
│   │   └── tmux_test.go             # tmux command tests
│   └── validate/
│       ├── validate.go              # session name validation
│       └── validate_test.go         # validation tests
├── go.mod
├── go.sum
├── README.md
└── .goreleaser.yaml
```

---

### Task 1: Initialize Go Module And Dependencies

**Files:**
- Create: `go.mod`
- Create: `main.go`

- [ ] **Step 1: Initialize the Go module**

Run: `cd /Users/hien/Repositories/devbox && go mod init github.com/hien/devbox`
Expected: `go.mod` created

- [ ] **Step 2: Add dependencies**

Run: `go get github.com/spf13/cobra@latest && go get gopkg.in/yaml.v3@latest && go get github.com/charmbracelet/bubbletea@latest && go get github.com/charmbracelet/lipgloss@latest`
Expected: `go.mod` and `go.sum` updated with all four deps

- [ ] **Step 3: Create the entry point**

Create `main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/hien/devbox/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create a minimal root command so it compiles**

Create `cmd/root.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "devbox",
	Short:   "Manage remote tmux sessions over SSH",
	Version: version,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Will be implemented in Task 5
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
}

func Execute() error {
	return rootCmd.Execute()
}
```

- [ ] **Step 5: Verify it builds**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 6: Commit**

```bash
git init
git add main.go cmd/root.go go.mod go.sum
git commit -m "feat: initialize devbox Go module with cobra"
```

---

### Task 2: Session Name Validation

**Files:**
- Create: `internal/validate/validate.go`
- Create: `internal/validate/validate_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/validate/validate_test.go`:

```go
package validate

import "testing"

func TestSessionName_Valid(t *testing.T) {
	valid := []string{"work", "api", "my-session", "test_123", "a"}
	for _, name := range valid {
		if err := SessionName(name); err != nil {
			t.Errorf("SessionName(%q) should be valid, got: %v", name, err)
		}
	}
}

func TestSessionName_Invalid(t *testing.T) {
	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"hello world", "spaces"},
		{"my/session", "slash"},
		{"name@host", "at sign"},
		{"a.b", "dot"},
		{string(make([]byte, 51)), "too long"},
	}
	for _, tc := range invalid {
		if err := SessionName(tc.name); err == nil {
			t.Errorf("SessionName(%q) should fail (%s), got nil", tc.name, tc.reason)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/validate/`
Expected: FAIL — `SessionName` not defined

- [ ] **Step 3: Write minimal implementation**

Create `internal/validate/validate.go`:

```go
package validate

import (
	"fmt"
	"regexp"
)

var sessionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func SessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf("session name too long (max 50 characters)")
	}
	if !sessionNamePattern.MatchString(name) {
		return fmt.Errorf("session name %q contains invalid characters (use alphanumeric, hyphens, underscores)", name)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/validate/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/validate/
git commit -m "feat: add session name validation"
```

---

### Task 3: Tmux Command Builders

**Files:**
- Create: `internal/tmux/tmux.go`
- Create: `internal/tmux/tmux_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tmux/tmux_test.go`:

```go
package tmux

import "testing"

func TestNewAttachSession(t *testing.T) {
	got := NewAttachSession("work")
	want := "tmux new -A -s work"
	if got != want {
		t.Errorf("NewAttachSession(work) = %q, want %q", got, want)
	}
}

func TestListSessions(t *testing.T) {
	got := ListSessions()
	want := "tmux list-sessions"
	if got != want {
		t.Errorf("ListSessions() = %q, want %q", got, want)
	}
}

func TestKillSession(t *testing.T) {
	got := KillSession("api")
	want := "tmux kill-session -t api"
	if got != want {
		t.Errorf("KillSession(api) = %q, want %q", got, want)
	}
}

func TestRenameSession(t *testing.T) {
	got := RenameSession("old", "new")
	want := "tmux rename-session -t old new"
	if got != want {
		t.Errorf("RenameSession(old, new) = %q, want %q", got, want)
	}
}

func TestListWindows(t *testing.T) {
	got := ListWindows("work")
	want := "tmux list-windows -t work"
	if got != want {
		t.Errorf("ListWindows(work) = %q, want %q", got, want)
	}
}

func TestHasSession(t *testing.T) {
	got := HasSession("api")
	want := "tmux has-session -t api"
	if got != want {
		t.Errorf("HasSession(api) = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/tmux/`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write minimal implementation**

Create `internal/tmux/tmux.go`:

```go
package tmux

import "fmt"

func NewAttachSession(name string) string {
	return fmt.Sprintf("tmux new -A -s %s", name)
}

func ListSessions() string {
	return "tmux list-sessions"
}

func KillSession(name string) string {
	return fmt.Sprintf("tmux kill-session -t %s", name)
}

func RenameSession(oldName, newName string) string {
	return fmt.Sprintf("tmux rename-session -t %s %s", oldName, newName)
}

func ListWindows(name string) string {
	return fmt.Sprintf("tmux list-windows -t %s", name)
}

func HasSession(name string) string {
	return fmt.Sprintf("tmux has-session -t %s", name)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/tmux/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/
git commit -m "feat: add tmux command builders"
```

---

### Task 4: SSH Executor Interface, Real Implementation, And Mock

**Files:**
- Create: `internal/ssh/executor.go`
- Create: `internal/ssh/ssh.go`
- Create: `internal/ssh/ssh_test.go`
- Create: `internal/ssh/mock.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ssh/ssh_test.go`:

```go
package ssh

import "testing"

func TestBuildArgs_BasicHost(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-p", "22", "hien@builder-104", "tmux list-sessions"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_Interactive(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "tmux new -A -s work")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-p", "22", "hien@builder-104", "tmux new -A -s work"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_NoCommand(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-p", "22", "hien@builder-104"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_CustomPort(t *testing.T) {
	c := Client{Host: "myhost", User: "admin", Port: 2222}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-p", "2222", "admin@myhost", "tmux list-sessions"}
	assertSliceEqual(t, got, want)
}

func assertSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("length mismatch: got %v, want %v", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q\n  full got:  %v\n  full want: %v", i, got[i], want[i], got, want)
			return
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/ssh/`
Expected: FAIL — `Client` and `buildArgs` not defined

- [ ] **Step 3: Write the Executor interface**

Create `internal/ssh/executor.go`:

```go
package ssh

// Executor abstracts SSH command execution for testability.
// Commands pass remote command strings (e.g. "tmux list-sessions");
// the implementation handles SSH arg construction internally.
type Executor interface {
	// Run executes a non-interactive SSH command and captures output.
	Run(remoteCmd string) (string, error)

	// Exec replaces the current process with SSH (interactive mode).
	Exec(remoteCmd string) error
}
```

- [ ] **Step 4: Write the real Client implementation**

Create `internal/ssh/ssh.go`:

```go
package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Client struct {
	Host    string
	User    string
	Port    int
	Verbose bool
}

func (c *Client) buildArgs(interactive bool, remoteCmd string) []string {
	var args []string
	if interactive {
		args = append(args, "-t")
	}
	args = append(args, "-o", "ConnectTimeout=10")
	args = append(args, "-p", fmt.Sprintf("%d", c.Port))
	args = append(args, fmt.Sprintf("%s@%s", c.User, c.Host))
	if remoteCmd != "" {
		args = append(args, remoteCmd)
	}
	return args
}

func (c *Client) Run(remoteCmd string) (string, error) {
	args := c.buildArgs(false, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (c *Client) Exec(remoteCmd string) error {
	args := c.buildArgs(true, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}
	argv := append([]string{"ssh"}, args...)
	return syscall.Exec(sshPath, argv, os.Environ())
}
```

- [ ] **Step 5: Write the mock**

Create `internal/ssh/mock.go`:

```go
package ssh

import "strings"

type MockExecutor struct {
	RunCalls  []string
	ExecCalls []string
	RunOutput string
	RunErr    error
	ExecErr   error
}

func (m *MockExecutor) Run(remoteCmd string) (string, error) {
	m.RunCalls = append(m.RunCalls, remoteCmd)
	return m.RunOutput, m.RunErr
}

func (m *MockExecutor) Exec(remoteCmd string) error {
	m.ExecCalls = append(m.ExecCalls, remoteCmd)
	return m.ExecErr
}

func (m *MockExecutor) LastRunContains(substr string) bool {
	if len(m.RunCalls) == 0 {
		return false
	}
	return strings.Contains(m.RunCalls[len(m.RunCalls)-1], substr)
}

func (m *MockExecutor) LastExecContains(substr string) bool {
	if len(m.ExecCalls) == 0 {
		return false
	}
	return strings.Contains(m.ExecCalls[len(m.ExecCalls)-1], substr)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/ssh/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ssh/
git commit -m "feat: add SSH executor interface with real and mock implementations"
```

---

### Task 5: Config Read/Write

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Host:           "builder-104",
		User:           "hien",
		Port:           22,
		DefaultSession: "work",
	}

	path := filepath.Join(dir, "config.yaml")
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Host != "builder-104" {
		t.Errorf("Host = %q, want %q", loaded.Host, "builder-104")
	}
	if loaded.User != "hien" {
		t.Errorf("User = %q, want %q", loaded.User, "hien")
	}
	if loaded.Port != 22 {
		t.Errorf("Port = %d, want %d", loaded.Port, 22)
	}
	if loaded.DefaultSession != "work" {
		t.Errorf("DefaultSession = %q, want %q", loaded.DefaultSession, "work")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load should fail for missing file")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("host: myhost\n"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 22 {
		t.Errorf("Port default = %d, want 22", cfg.Port)
	}
	if cfg.DefaultSession != "work" {
		t.Errorf("DefaultSession default = %q, want %q", cfg.DefaultSession, "work")
	}
}

func TestConfigDir(t *testing.T) {
	dir := t.TempDir()
	original := configDirOverride
	configDirOverride = dir
	defer func() { configDirOverride = original }()

	got := Dir()
	if got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/config/`
Expected: FAIL — `Config`, `Save`, `Load` not defined

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host           string `yaml:"host"`
	User           string `yaml:"user,omitempty"`
	Port           int    `yaml:"port,omitempty"`
	DefaultSession string `yaml:"default_session,omitempty"`
}

var configDirOverride string

func Dir() string {
	if configDirOverride != "" {
		return configDirOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "devbox")
}

func SetDirOverride(dir string) {
	configDirOverride = dir
}

func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no config found: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.DefaultSession == "" {
		cfg.DefaultSession = "work"
	}
	return cfg, nil
}

func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot serialize config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func LoadDefault() (*Config, error) {
	return Load(Path())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add config read/write with defaults"
```

---

### Task 6: Per-Terminal Session Tracking

**Files:**
- Create: `internal/config/session.go`
- Create: `internal/config/session_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/session_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndGetSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")

	if err := SaveSession(sessDir, "ttys003", "work"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	name, err := GetSession(sessDir, "ttys003")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if name != "work" {
		t.Errorf("GetSession = %q, want %q", name, "work")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := GetSession(dir, "ttys999")
	if err == nil {
		t.Error("GetSession should fail for unknown TTY")
	}
}

func TestDeleteSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	SaveSession(sessDir, "ttys003", "work")
	DeleteSession(sessDir, "ttys003")

	_, err := GetSession(sessDir, "ttys003")
	if err == nil {
		t.Error("GetSession should fail after delete")
	}
}

func TestLastSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")

	SaveSession(sessDir, "ttys001", "api")
	time.Sleep(10 * time.Millisecond)
	SaveSession(sessDir, "ttys002", "work")

	name, err := LastSession(sessDir)
	if err != nil {
		t.Fatalf("LastSession: %v", err)
	}
	if name != "work" {
		t.Errorf("LastSession = %q, want %q", name, "work")
	}
}

func TestLastSession_Empty(t *testing.T) {
	dir := t.TempDir()
	_, err := LastSession(dir)
	if err == nil {
		t.Error("LastSession should fail with no sessions")
	}
}

func TestResolveTTY(t *testing.T) {
	name, err := ResolveTTY()
	if err != nil {
		t.Skipf("no TTY available: %v", err)
	}
	if name == "" {
		t.Error("ResolveTTY returned empty string")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/config/`
Expected: FAIL — `SaveSession`, `GetSession`, etc. not defined

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/session.go`:

```go
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func SessionsDir() string {
	return filepath.Join(Dir(), "sessions")
}

func SaveSession(sessDir, ttyName, sessionName string) error {
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(sessDir, ttyName)
	return os.WriteFile(path, []byte(sessionName), 0644)
}

func GetSession(sessDir, ttyName string) (string, error) {
	path := filepath.Join(sessDir, ttyName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no session for this terminal")
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", fmt.Errorf("empty session name")
	}
	return name, nil
}

func DeleteSession(sessDir, ttyName string) {
	path := filepath.Join(sessDir, ttyName)
	os.Remove(path)
}

func LastSession(sessDir string) (string, error) {
	entries, err := os.ReadDir(sessDir)
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("no session history")
	}

	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UnixNano() > newestTime {
			newestTime = info.ModTime().UnixNano()
			path := filepath.Join(sessDir, e.Name())
			data, err := os.ReadFile(path)
			if err == nil {
				newest = strings.TrimSpace(string(data))
			}
		}
	}

	if newest == "" {
		return "", fmt.Errorf("no session history")
	}
	return newest, nil
}

func ResolveTTY() (string, error) {
	out, err := exec.Command("tty").Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine terminal: %w", err)
	}
	ttyPath := strings.TrimSpace(string(out))
	// /dev/ttys003 -> ttys003
	return filepath.Base(ttyPath), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/hien/Repositories/devbox && go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/session.go internal/config/session_test.go
git commit -m "feat: add per-terminal session tracking"
```

---

### Task 7: `devbox connect` Command

**Files:**
- Create: `cmd/connect.go`

- [ ] **Step 1: Write the command**

Create `cmd/connect.go`:

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/hien/devbox/internal/config"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Configure SSH host for devbox",
	RunE:  runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Host (required)
	fmt.Print("SSH host: ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is required")
	}

	// User (optional)
	currentUser, _ := user.Current()
	defaultUser := currentUser.Username
	fmt.Printf("SSH user [%s]: ", defaultUser)
	sshUser, _ := reader.ReadString('\n')
	sshUser = strings.TrimSpace(sshUser)
	if sshUser == "" {
		sshUser = defaultUser
	}

	// Port (optional)
	fmt.Print("SSH port [22]: ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	port := 22
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port: %s", portStr)
		}
		port = p
	}

	// Test connection
	fmt.Printf("Testing connection to %s@%s:%d...\n", sshUser, host, port)
	testCmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf("%s@%s", sshUser, host),
		"echo", "ok",
	)
	testCmd.Stderr = os.Stderr
	out, err := testCmd.Output()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		fmt.Print("Save anyway? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
	} else {
		if strings.TrimSpace(string(out)) == "ok" {
			fmt.Println("Connection successful!")
		}
	}

	cfg := &config.Config{
		Host:           host,
		User:           sshUser,
		Port:           port,
		DefaultSession: "work",
	}

	if err := config.Save(cfg, config.Path()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Config saved to %s\n", config.Path())
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 3: Commit**

```bash
git add cmd/connect.go
git commit -m "feat: add devbox connect command"
```

---

### Task 8: Root Command — Default Attach Behavior

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Update root command to attach default session**

Update `cmd/root.go` `RunE` to load config, resolve TTY, save session mapping, and exec SSH:

```go
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/hien/devbox/internal/config"
	sshpkg "github.com/hien/devbox/internal/ssh"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "devbox",
	Short:   "Manage remote tmux sessions over SSH",
	Version: version,
	RunE:    runRoot,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
}

func Execute() error {
	return rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	sessionName := cfg.DefaultSession

	saveCurrentSession(sessionName)

	remoteCmd := tmux.NewAttachSession(sessionName)
	return client.Exec(remoteCmd)
}

func requireConfig() (*config.Config, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("no host configured. Run \"devbox connect\" first")
	}
	return config.LoadDefault()
}

func newClient(cfg *config.Config) *sshpkg.Client {
	return &sshpkg.Client{
		Host:    cfg.Host,
		User:    cfg.User,
		Port:    cfg.Port,
		Verbose: verbose,
	}
}

func saveCurrentSession(name string) {
	tty, err := config.ResolveTTY()
	if err != nil {
		return // silently skip if no TTY
	}
	config.SaveSession(config.SessionsDir(), tty, name)
}

func handleSSHError(err error, host string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "command not found") {
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox connect\"", host)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "feat: attach default tmux session on bare devbox"
```

---

### Task 9: `devbox list` Command

**Files:**
- Create: `cmd/list.go`

- [ ] **Step 1: Write the command**

Create `cmd/list.go`:

```go
package cmd

import (
	"fmt"
	"strings"

	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tmux sessions on the remote host",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if strings.Contains(output, "no server running") || strings.Contains(output, "no sessions") {
			fmt.Printf("No active sessions on %s.\n", cfg.Host)
			return nil
		}
		return handleSSHError(err, cfg.Host)
	}

	fmt.Println(output)
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 3: Commit**

```bash
git add cmd/list.go
git commit -m "feat: add devbox list command"
```

---

### Task 10: `devbox new` And `devbox resume` Commands

**Files:**
- Create: `cmd/new.go`
- Create: `cmd/resume.go`

- [ ] **Step 1: Write `devbox new`**

Create `cmd/new.go`:

```go
package cmd

import (
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new NAME",
	Short: "Create or attach to a named tmux session",
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validate.SessionName(name); err != nil {
		return err
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	saveCurrentSession(name)

	return client.Exec(tmux.NewAttachSession(name))
}
```

- [ ] **Step 2: Write `devbox resume`**

Create `cmd/resume.go`:

```go
package cmd

import (
	"fmt"
	"strings"

	"github.com/hien/devbox/internal/config"
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [NAME]",
	Short: "Attach to a session (by name, or resume this terminal's last session)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)

	// Named resume
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		saveCurrentSession(name)
		return client.Exec(tmux.NewAttachSession(name))
	}

	// Per-terminal resume
	tty, err := config.ResolveTTY()
	if err != nil {
		fmt.Println("Cannot determine terminal. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	sessionName, err := config.GetSession(config.SessionsDir(), tty)
	if err != nil {
		fmt.Println("No session for this terminal. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	// Verify session still exists on remote
	_, checkErr := client.Run(tmux.HasSession(sessionName))
	if checkErr != nil {
		config.DeleteSession(config.SessionsDir(), tty)
		fmt.Printf("Session %q no longer exists. Launching session picker...\n", sessionName)
		return runAttachPicker(cfg, client)
	}

	saveCurrentSession(sessionName)
	return client.Exec(tmux.NewAttachSession(sessionName))
}

// runAttachPicker is a forward reference — implemented in cmd/attach.go.
// For now, stub it so the code compiles. Task 14 replaces this.
func runAttachPicker(cfg *config.Config, client *sshpkg.Client) error {
	// Fetch sessions
	output, err := client.Run(tmux.ListSessions())
	if err != nil || strings.TrimSpace(output) == "" {
		fmt.Println("No sessions available. Creating default session...")
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	// Simple fallback: print list and pick first
	// Will be replaced with bubbletea picker in Task 14
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	// Parse first session name (format: "name: N windows ...")
	first := strings.SplitN(lines[0], ":", 2)[0]
	fmt.Printf("Attaching to %q...\n", first)
	saveCurrentSession(first)
	return client.Exec(tmux.NewAttachSession(first))
}
```

Note: `runAttachPicker` is a temporary stub. Task 14 replaces it with the bubbletea picker. Add the missing import for `sshpkg`:

```go
import sshpkg "github.com/hien/devbox/internal/ssh"
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 4: Commit**

```bash
git add cmd/new.go cmd/resume.go
git commit -m "feat: add devbox new and resume commands"
```

---

### Task 11: `devbox kill` And `devbox rename` Commands

**Files:**
- Create: `cmd/kill.go`
- Create: `cmd/rename.go`

- [ ] **Step 1: Write `devbox kill`**

Create `cmd/kill.go`:

```go
package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill NAME",
	Short: "Kill a tmux session by name",
	Args:  cobra.ExactArgs(1),
	RunE:  runKill,
}

func init() {
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validate.SessionName(name); err != nil {
		return err
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	output, err := client.Run(tmux.KillSession(name))
	if err != nil {
		return fmt.Errorf("kill session %q: %s", name, output)
	}

	fmt.Printf("Session %q killed.\n", name)
	return nil
}
```

- [ ] **Step 2: Write `devbox rename`**

Create `cmd/rename.go`:

```go
package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename OLD NEW",
	Short: "Rename a tmux session",
	Args:  cobra.ExactArgs(2),
	RunE:  runRename,
}

func init() {
	rootCmd.AddCommand(renameCmd)
}

func runRename(cmd *cobra.Command, args []string) error {
	oldName, newName := args[0], args[1]
	if err := validate.SessionName(oldName); err != nil {
		return fmt.Errorf("old name: %w", err)
	}
	if err := validate.SessionName(newName); err != nil {
		return fmt.Errorf("new name: %w", err)
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	output, err := client.Run(tmux.RenameSession(oldName, newName))
	if err != nil {
		return fmt.Errorf("rename session: %s", output)
	}

	fmt.Printf("Session %q renamed to %q.\n", oldName, newName)
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 4: Commit**

```bash
git add cmd/kill.go cmd/rename.go
git commit -m "feat: add devbox kill and rename commands"
```

---

### Task 12: `devbox shell` Command

**Files:**
- Create: `cmd/shell.go`

- [ ] **Step 1: Write the command**

Create `cmd/shell.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a raw SSH shell (no tmux)",
	RunE:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	return client.Exec("")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 3: Commit**

```bash
git add cmd/shell.go
git commit -m "feat: add devbox shell command"
```

---

### Task 13: `devbox last` Command

**Files:**
- Create: `cmd/last.go`

- [ ] **Step 1: Write the command**

Create `cmd/last.go`:

```go
package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/config"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var lastCmd = &cobra.Command{
	Use:   "last",
	Short: "Resume the most recently used session (global)",
	RunE:  runLast,
}

func init() {
	rootCmd.AddCommand(lastCmd)
}

func runLast(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)

	sessionName, err := config.LastSession(config.SessionsDir())
	if err != nil {
		fmt.Println("No session history. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	// Verify session still exists
	_, checkErr := client.Run(tmux.HasSession(sessionName))
	if checkErr != nil {
		fmt.Printf("Session %q no longer exists. Launching session picker...\n", sessionName)
		return runAttachPicker(cfg, client)
	}

	fmt.Printf("Resuming %q...\n", sessionName)
	saveCurrentSession(sessionName)
	return client.Exec(tmux.NewAttachSession(sessionName))
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 3: Commit**

```bash
git add cmd/last.go
git commit -m "feat: add devbox last command"
```

---

### Task 14: `devbox attach` — Bubbletea Interactive Picker

**Files:**
- Create: `cmd/attach.go`
- Modify: `cmd/resume.go` (replace `runAttachPicker` stub)

- [ ] **Step 1: Write the bubbletea picker**

Create `cmd/attach.go`:

```go
package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hien/devbox/internal/config"
	sshpkg "github.com/hien/devbox/internal/ssh"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Interactive session picker",
	RunE:  runAttach,
}

func init() {
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	client := newClient(cfg)
	return runAttachPicker(cfg, client)
}

type sessionItem struct {
	name    string
	details string
}

type pickerModel struct {
	sessions []sessionItem
	cursor   int
	chosen   string
	quitting bool
}

func (m pickerModel) Init() bubbletea.Cmd {
	return nil
}

func (m pickerModel) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, bubbletea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.sessions[m.cursor].name
			return m, bubbletea.Quit
		}
	}
	return m, nil
}

var (
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	unselectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	headerStyle     = lipgloss.NewStyle().Bold(true).MarginBottom(1)
)

func (m pickerModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Select a session:"))
	b.WriteString("\n\n")

	for i, s := range m.sessions {
		cursor := "  "
		style := unselectedStyle
		if m.cursor == i {
			cursor = "> "
			style = selectedStyle
		}
		line := fmt.Sprintf("%s%s", cursor, s.name)
		if s.details != "" {
			line += fmt.Sprintf("  %s", s.details)
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(unselectedStyle.Render("↑/↓ navigate • enter select • q quit"))
	return b.String()
}

func parseSessions(output string) []sessionItem {
	var items []sessionItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// tmux list-sessions format: "name: N windows (created ...)"
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		details := ""
		if len(parts) > 1 {
			details = strings.TrimSpace(parts[1])
		}
		items = append(items, sessionItem{name: name, details: details})
	}
	return items
}
```

- [ ] **Step 2: Remove `runAttachPicker` stub from resume.go**

In `cmd/resume.go`, remove the `runAttachPicker` function entirely (it's now in `attach.go`). Keep the `runResume` function which calls `runAttachPicker`. Move `runAttachPicker` to `cmd/attach.go` as the real implementation:

Add this function to the bottom of `cmd/attach.go`:

```go
func runAttachPicker(cfg *config.Config, client *sshpkg.Client) error {
	output, err := client.Run(tmux.ListSessions())
	if err != nil || strings.TrimSpace(output) == "" {
		fmt.Println("No sessions available. Creating default session...")
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	sessions := parseSessions(output)
	if len(sessions) == 0 {
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	model := pickerModel{sessions: sessions}
	p := bubbletea.NewProgram(model)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	final := result.(pickerModel)
	if final.quitting || final.chosen == "" {
		return nil // user cancelled
	}

	saveCurrentSession(final.chosen)
	return client.Exec(tmux.NewAttachSession(final.chosen))
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/hien/Repositories/devbox && go build -o /dev/null .`
Expected: builds with exit code 0

- [ ] **Step 4: Commit**

```bash
git add cmd/attach.go cmd/resume.go
git commit -m "feat: add interactive session picker with bubbletea"
```

---

### Task 15: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

Create `README.md`:

```markdown
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
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add devbox usage guide"
```

---

### Task 16: GoReleaser Config

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Write GoReleaser config**

Create `.goreleaser.yaml`:

```yaml
version: 2
builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X github.com/hien/devbox/cmd.version={{.Version}}

archives:
  - format: tar.gz
    name_template: "devbox_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
```

- [ ] **Step 2: Commit**

```bash
git add .goreleaser.yaml
git commit -m "build: add GoReleaser config for cross-platform releases"
```

---

### Task 17: Run Full Test Suite And Manual Verification

**Files:**
- Verify: all files

- [ ] **Step 1: Run all tests**

Run: `cd /Users/hien/Repositories/devbox && go test ./...`
Expected: PASS on all packages

- [ ] **Step 2: Build the binary**

Run: `cd /Users/hien/Repositories/devbox && go build -o ./devbox .`
Expected: `devbox` binary created

- [ ] **Step 3: Verify help output**

Run: `./devbox help`
Expected: lists all subcommands (connect, list, new, resume, kill, rename, shell, last, attach)

- [ ] **Step 4: Run devbox connect against real host**

Run: `./devbox connect`
Expected: prompts for host/user/port, tests connection, saves config

- [ ] **Step 5: Test session workflow**

```bash
./devbox --verbose            # attach default session
# detach with Ctrl+B, D
./devbox list                 # shows sessions
./devbox new api --verbose    # create api session
# detach
./devbox resume               # should resume api (per-terminal)
# detach
./devbox last                 # should resume most recent
./devbox rename api backend
./devbox list
./devbox kill backend
./devbox shell                # raw SSH
./devbox attach               # interactive picker
```

- [ ] **Step 6: Clean up build artifact**

Run: `rm ./devbox`

- [ ] **Step 7: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: post-verification fixes"
```

---

## Notes For Implementation

- Use `strings.Contains` for error message detection — keep it simple.
- The `runAttachPicker` function in `cmd/attach.go` is shared by `resume`, `last`, and `attach` commands. It must be defined once in `attach.go` and referenced from the others.
- When `syscall.Exec` succeeds, the process is replaced — no code after it runs. Handle all pre-exec work (session tracking, verbose logging) before the `Exec` call.
- Session name validation happens in `cmd/` layer before passing to `internal/` packages.
- The `connect` command is the only command that doesn't go through the `requireConfig()` gate.

## Testing Strategy

- Primary: `go test ./...`
- Manual: real SSH host with tmux
- Focus areas: SSH arg construction, config defaults, session file lifecycle, session name validation

## Definition Of Done

- `devbox` binary builds for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
- All subcommands work against a real remote host with tmux
- `devbox connect` saves config and verifies connectivity
- Per-terminal session tracking works across multiple terminal tabs
- `devbox attach` shows bubbletea interactive picker
- `go test ./...` passes
- README documents install, setup, and all commands
- `.goreleaser.yaml` configured for GitHub Releases
