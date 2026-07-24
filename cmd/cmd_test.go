package cmd

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hien/devbox/internal/config"
	sshpkg "github.com/hien/devbox/internal/ssh"
)

// exitWith produces a real *exec.ExitError with the given code.
func exitWith(t *testing.T, code int) error {
	t.Helper()
	return exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
}

func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config.SetDirOverride(dir)
	t.Cleanup(func() { config.SetDirOverride("") })
	return dir
}

// --- parseSessions ---

func TestParseSessions(t *testing.T) {
	output := "work: 3 windows (created Mon Jul 20 10:00:00 2026) (attached)\napi: 1 windows (created Tue Jul 21 09:00:00 2026)"
	items := parseSessions(output)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].name != "work" || items[1].name != "api" {
		t.Errorf("names = %q, %q; want work, api", items[0].name, items[1].name)
	}
	if !strings.Contains(items[0].details, "3 windows") {
		t.Errorf("details = %q, want window info", items[0].details)
	}
}

func TestParseSessions_EmptyAndBlankLines(t *testing.T) {
	if items := parseSessions(""); len(items) != 0 {
		t.Errorf("empty output: got %d items, want 0", len(items))
	}
	if items := parseSessions("\n  \nwork: 1 windows\n\n"); len(items) != 1 {
		t.Errorf("blank lines: got %d items, want 1", len(items))
	}
}

// --- error classification ---

func TestHandleSSHError_TmuxMissingFromOutput(t *testing.T) {
	// "command not found" lives in the captured output, not err.Error()
	err := handleSSHError("bash: line 1: tmux: command not found", exitWith(t, 127), "myhost")
	if err == nil || !strings.Contains(err.Error(), "tmux is not installed") {
		t.Errorf("got %v, want tmux-not-installed error", err)
	}
}

func TestHandleSSHError_Unreachable(t *testing.T) {
	err := handleSSHError("ssh: connect to host myhost port 22: Connection refused", exitWith(t, 255), "myhost")
	if err == nil || !strings.Contains(err.Error(), "cannot reach myhost") {
		t.Errorf("got %v, want cannot-reach error", err)
	}
}

func TestHandleSSHError_NilError(t *testing.T) {
	if err := handleSSHError("anything", nil, "myhost"); err != nil {
		t.Errorf("nil err should stay nil, got %v", err)
	}
}

func TestHandleExecErr_MapsExit127ToTmuxMissing(t *testing.T) {
	err := handleExecErr(exitWith(t, 127), "myhost")
	if err == nil || !strings.Contains(err.Error(), "tmux is not installed") {
		t.Errorf("got %v, want tmux-not-installed error", err)
	}
}

func TestHandleExecErr_PassthroughOtherErrors(t *testing.T) {
	if err := handleExecErr(nil, "myhost"); err != nil {
		t.Errorf("nil should stay nil, got %v", err)
	}
	orig := exitWith(t, 1)
	if err := handleExecErr(orig, "myhost"); err == nil {
		t.Error("exit 1 should surface an error")
	}
}

// --- root command ---

func TestRoot_RejectsStrayArgs(t *testing.T) {
	// "devbox tpyo" must error, not silently attach to the default session
	if rootCmd.Args == nil {
		t.Fatal("rootCmd.Args is nil; stray args are accepted")
	}
	if err := rootCmd.Args(rootCmd, []string{"tpyo"}); err == nil {
		t.Error("stray arg should be rejected")
	}
	if err := rootCmd.Args(rootCmd, nil); err != nil {
		t.Errorf("no args should be accepted, got %v", err)
	}
}

// --- killSession ---

func TestKillSession_RunsKillAndCleansAffinity(t *testing.T) {
	dir := useTempConfigDir(t)
	sessDir := filepath.Join(dir, "sessions")
	config.SaveSession(sessDir, "ttys001", "api")
	config.SaveSession(sessDir, "ttys002", "work")

	mock := &sshpkg.MockExecutor{}
	if err := killSession(mock, "myhost", "api"); err != nil {
		t.Fatalf("killSession: %v", err)
	}
	if !mock.LastRunContains("kill-session") || !mock.LastRunContains("api") {
		t.Errorf("expected kill-session for api, got %v", mock.RunCalls)
	}
	if _, err := config.GetSession(sessDir, "ttys001"); err == nil {
		t.Error("affinity record for killed session should be removed")
	}
	if name, _ := config.GetSession(sessDir, "ttys002"); name != "work" {
		t.Error("unrelated affinity record should survive")
	}
}

// --- kill picker (async) ---

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func newTestKillModel(mock *sshpkg.MockExecutor) killPickerModel {
	return killPickerModel{
		client:   mock,
		host:     "myhost",
		sessions: []sessionItem{{name: "api"}, {name: "work"}},
	}
}

func TestKillPicker_XKillsAsync(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	m := newTestKillModel(mock)

	// "x" must NOT block: it returns a tea.Cmd and marks the kill in flight
	updated, cmd := m.Update(keyRune('x'))
	m = updated.(killPickerModel)
	if cmd == nil {
		t.Fatal("expected async tea.Cmd, got nil")
	}
	if len(mock.RunCalls) != 0 {
		t.Fatal("Update must not call SSH synchronously")
	}

	// Running the command performs the SSH kill off the UI thread
	msg := cmd()
	if !mock.LastRunContains("kill-session") {
		t.Fatalf("expected kill-session call, got %v", mock.RunCalls)
	}

	// Feeding the result back removes the session and records it
	updated, _ = m.Update(msg)
	m = updated.(killPickerModel)
	if len(m.sessions) != 1 || m.sessions[0].name != "work" {
		t.Errorf("sessions after kill = %v, want [work]", m.sessions)
	}
	if len(m.killed) != 1 || m.killed[0] != "api" {
		t.Errorf("killed = %v, want [api]", m.killed)
	}
}

func TestKillPicker_IgnoresKeysWhileKillInFlight(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	m := newTestKillModel(mock)

	updated, _ := m.Update(keyRune('x'))
	m = updated.(killPickerModel)

	// Second "x" before the result arrives must not start another kill
	_, cmd2 := m.Update(keyRune('x'))
	if cmd2 != nil {
		t.Error("second x while in flight should be ignored")
	}
}

func TestKillPicker_EscWhileInflightDefersQuitUntilResult(t *testing.T) {
	// Quitting mid-kill must not drop the result: the kill already ran on
	// the server, so affinity cleanup and reporting still have to happen
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	m := newTestKillModel(mock)

	updated, cmd := m.Update(keyRune('x'))
	m = updated.(killPickerModel)

	updated, quitCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(killPickerModel)
	if m.quitting || quitCmd != nil {
		t.Fatal("esc during in-flight kill must defer quit, not quit immediately")
	}

	updated, quitCmd = m.Update(cmd())
	m = updated.(killPickerModel)
	if !m.quitting || quitCmd == nil {
		t.Error("model should quit once the pending result is processed")
	}
	if len(m.killed) != 1 || m.killed[0] != "api" {
		t.Errorf("deferred result must still be recorded, got %v", m.killed)
	}
}

func TestKillPicker_FailedKillKeepsSession(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{RunErr: exitWith(t, 1), RunOutput: "can't find session"}
	m := newTestKillModel(mock)

	updated, cmd := m.Update(keyRune('x'))
	m = updated.(killPickerModel)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(killPickerModel)

	if len(m.sessions) != 2 {
		t.Errorf("failed kill must keep the session, got %v", m.sessions)
	}
	if len(m.killed) != 0 {
		t.Errorf("failed kill must not be recorded, got %v", m.killed)
	}
	if !strings.Contains(m.status, "Failed") {
		t.Errorf("status = %q, want failure notice", m.status)
	}
}

func TestKillPicker_EnterKillFailureIsRecorded(t *testing.T) {
	// A failed enter-kill must not exit silently with success
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{RunErr: exitWith(t, 1), RunOutput: "can't find session"}
	m := newTestKillModel(mock)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(killPickerModel)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(killPickerModel)

	if m.failed == "" {
		t.Error("failed enter-kill should record the failed session name")
	}
	if len(m.killed) != 0 {
		t.Errorf("failed kill must not appear in killed, got %v", m.killed)
	}
}

func TestKillPicker_EnterKillsAndQuits(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	m := newTestKillModel(mock)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(killPickerModel)
	if cmd == nil {
		t.Fatal("enter should trigger an async kill")
	}
	msg := cmd()
	updated, quitCmd := m.Update(msg)
	m = updated.(killPickerModel)
	if !m.quitting {
		t.Error("model should quit after enter-kill completes")
	}
	if quitCmd == nil {
		t.Error("expected tea.Quit after enter-kill completes")
	}
}

// --- attach picker list handling ---

func TestListSessionsForPicker_TransportErrorSurfaces(t *testing.T) {
	mock := &sshpkg.MockExecutor{
		RunErr:    exitWith(t, 255),
		RunOutput: "ssh: connect to host myhost port 22: No route to host",
	}
	_, err := listSessionsForPicker(mock, "myhost")
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("transport error must surface, got %v", err)
	}
}

func TestListSessionsForPicker_NoServerMeansEmpty(t *testing.T) {
	mock := &sshpkg.MockExecutor{
		RunErr:    exitWith(t, 1),
		RunOutput: "no server running on /private/tmp/tmux-501/default",
	}
	items, err := listSessionsForPicker(mock, "myhost")
	if err != nil {
		t.Fatalf("no-server is not an error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %v, want empty", items)
	}
}

func TestListSessionsForPicker_FreshServerMeansEmpty(t *testing.T) {
	// A box where the tmux server never started emits a different message
	// than "no server running" — it must still classify as empty, not as a
	// transport failure
	mock := &sshpkg.MockExecutor{
		RunErr:    exitWith(t, 1),
		RunOutput: "error connecting to /tmp/tmux-501/default (No such file or directory)",
	}
	items, err := listSessionsForPicker(mock, "myhost")
	if err != nil {
		t.Fatalf("fresh server is not an error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %v, want empty", items)
	}
}

func TestListSessionsForPicker_ParsesSessions(t *testing.T) {
	mock := &sshpkg.MockExecutor{RunOutput: "work: 1 windows\napi: 2 windows"}
	items, err := listSessionsForPicker(mock, "myhost")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
}

// --- connect: preserve customized default_session ---

func TestPreservedDefaultSession_KeepsExisting(t *testing.T) {
	useTempConfigDir(t)
	cfg := &config.Config{Host: "h", DefaultSession: "myproj"}
	if err := config.Save(cfg, config.Path()); err != nil {
		t.Fatal(err)
	}
	if got := preservedDefaultSession(); got != "myproj" {
		t.Errorf("got %q, want myproj", got)
	}
}

func TestPreservedDefaultSession_DefaultsToWork(t *testing.T) {
	useTempConfigDir(t)
	if got := preservedDefaultSession(); got != "work" {
		t.Errorf("got %q, want work", got)
	}
}

// --- shell: remote exit codes are the user's business ---

func TestRunShell_SwallowsRemoteExitCodes(t *testing.T) {
	useTempConfigDir(t)
	if err := config.Save(&config.Config{Host: "h"}, config.Path()); err != nil {
		t.Fatal(err)
	}
	mock := &sshpkg.MockExecutor{ExecErr: exitWith(t, 1)}
	orig := newClient
	newClient = func(cfg *config.Config) sshpkg.Executor { return mock }
	t.Cleanup(func() { newClient = orig })

	if err := runShell(shellCmd, nil); err != nil {
		t.Errorf("remote exit 1 should not be a devbox error, got %v", err)
	}
	if len(mock.ExecCalls) != 1 || mock.ExecCalls[0] != "" {
		t.Errorf("expected one raw-shell Exec, got %v", mock.ExecCalls)
	}
}

func TestRunShell_SurfacesSpawnFailure(t *testing.T) {
	useTempConfigDir(t)
	if err := config.Save(&config.Config{Host: "h"}, config.Path()); err != nil {
		t.Fatal(err)
	}
	mock := &sshpkg.MockExecutor{ExecErr: exec.Command("/nonexistent/ssh").Run()}
	orig := newClient
	newClient = func(cfg *config.Config) sshpkg.Executor { return mock }
	t.Cleanup(func() { newClient = orig })

	if err := runShell(shellCmd, nil); err == nil {
		t.Error("a non-exit error (ssh missing) must surface")
	}
}

// --- session id generation ---

func TestGenerateSessionID_Format(t *testing.T) {
	pattern := regexp.MustCompile(`^s-[0-9a-f]{8}$`)
	for i := 0; i < 5; i++ {
		id := generateSessionID()
		if !pattern.MatchString(id) {
			t.Errorf("id %q does not match %s", id, pattern)
		}
	}
}
