package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hiennguyenle/devbox/internal/config"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
	"github.com/hiennguyenle/devbox/internal/warp"
)

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

// saveConfig writes a config with the given profiles and marks the first as
// default, then returns it.
func saveConfig(t *testing.T, profiles map[string]*config.Profile, def string) {
	t.Helper()
	cfg := &config.Config{DefaultProfile: def, Profiles: profiles}
	if err := config.Save(cfg, config.Path()); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

// oneProfile is a config with a single profile that is also the default.
func oneProfile(name, host string) (map[string]*config.Profile, string) {
	return map[string]*config.Profile{name: {Host: host, DefaultSession: "work"}}, name
}

// fakeTTY makes config.ResolveTTY return a fixed name, so per-terminal logic is
// deterministic in tests that would otherwise skip for lack of a real TTY.
func fakeTTY(t *testing.T, name string) {
	t.Helper()
	orig := config.ResolveTTY
	config.ResolveTTY = func() (string, error) { return name, nil }
	t.Cleanup(func() { config.ResolveTTY = orig })
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	os.Stdout = original
	t.Cleanup(func() {
		os.Stdout = original
		r.Close()
	})

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
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
	err := handleSSHError("bash: line 1: tmux: command not found", exitWith(t, 127), "myhost", false)
	if err == nil || !strings.Contains(err.Error(), "tmux is not installed") {
		t.Errorf("got %v, want tmux-not-installed error", err)
	}
}

func TestHandleSSHError_Unreachable(t *testing.T) {
	err := handleSSHError("ssh: connect to host myhost port 22: Connection refused", exitWith(t, 255), "myhost", false)
	if err == nil || !strings.Contains(err.Error(), "cannot reach myhost") {
		t.Errorf("got %v, want cannot-reach error", err)
	}
}

func TestHandleSSHError_NilError(t *testing.T) {
	if err := handleSSHError("anything", nil, "myhost", false); err != nil {
		t.Errorf("nil err should stay nil, got %v", err)
	}
}

func TestHandleExecErr_MapsExit127ToTmuxMissing(t *testing.T) {
	err := handleExecErr(exitWith(t, 127), "myhost", false)
	if err == nil || !strings.Contains(err.Error(), "tmux is not installed") {
		t.Errorf("got %v, want tmux-not-installed error", err)
	}
}

func TestHandleExecErr_PassthroughOtherErrors(t *testing.T) {
	if err := handleExecErr(nil, "myhost", false); err != nil {
		t.Errorf("nil should stay nil, got %v", err)
	}
	orig := exitWith(t, 1)
	if err := handleExecErr(orig, "myhost", false); err == nil {
		t.Error("exit 1 should surface an error")
	}
}

func TestExecTmuxSession_PrintsResumeHintOnCleanExit(t *testing.T) {
	mock := &sshpkg.MockExecutor{}
	var runErr error

	out := captureStdout(t, func() {
		runErr = execTmuxSession(mock, "builder", "myhost", "api")
	})

	if runErr != nil {
		t.Fatalf("execTmuxSession: %v", runErr)
	}
	if want := "To continue this session, run devbox resume api\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !mock.LastExecContains("api") {
		t.Errorf("Exec calls = %v, want attach to api", mock.ExecCalls)
	}
}

func TestExecTmuxSession_PrintsResumeHintOnceOnDisconnect(t *testing.T) {
	mock := &sshpkg.MockExecutor{DisconnectOnExec: true}
	var runErr error

	out := captureStdout(t, func() {
		runErr = execTmuxSession(mock, "builder", "myhost", "api")
	})

	if runErr != nil {
		t.Fatalf("execTmuxSession: %v", runErr)
	}
	if want := "To continue this session, run devbox resume api\n"; out != want {
		t.Errorf("stdout = %q, want one disconnect hint %q", out, want)
	}
}

func TestExecTmuxSession_DoesNotPrintHintOnFailure(t *testing.T) {
	mock := &sshpkg.MockExecutor{ExecErr: exitWith(t, 127)}
	var runErr error

	out := captureStdout(t, func() {
		runErr = execTmuxSession(mock, "builder", "myhost", "api")
	})

	if runErr == nil || !strings.Contains(runErr.Error(), "tmux is not installed") {
		t.Fatalf("error = %v, want tmux-not-installed error", runErr)
	}
	if out != "" {
		t.Errorf("stdout = %q, want no resume hint", out)
	}
}

func TestExecTmuxSession_InvalidDirectResumeNamePrintsPickerFallback(t *testing.T) {
	mock := &sshpkg.MockExecutor{}
	var runErr error

	out := captureStdout(t, func() {
		runErr = execTmuxSession(mock, "builder", "myhost", "out of band")
	})

	if runErr != nil {
		t.Fatalf("execTmuxSession: %v", runErr)
	}
	if want := "To continue this session, run devbox resume\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// --- root command ---

func TestRoot_AcceptsAtMostOneProfileArg(t *testing.T) {
	if rootCmd.Args == nil {
		t.Fatal("rootCmd.Args is nil")
	}
	if err := rootCmd.Args(rootCmd, nil); err != nil {
		t.Errorf("no args should be accepted, got %v", err)
	}
	if err := rootCmd.Args(rootCmd, []string{"builder-104"}); err != nil {
		t.Errorf("one profile arg should be accepted, got %v", err)
	}
	if err := rootCmd.Args(rootCmd, []string{"a", "b"}); err == nil {
		t.Error("two args should be rejected")
	}
}

func TestRunRoot_UnknownProfileErrors(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("builder-104", "h")
	saveConfig(t, p, def)

	orig := newClient
	newClient = func(*config.Profile) sshpkg.Executor { return &sshpkg.MockExecutor{} }
	t.Cleanup(func() { newClient = orig })

	if err := runRoot(rootCmd, []string{"nope"}); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("got %v, want unknown-profile error", err)
	}
}

func TestRunRoot_AttachesResolvedProfileAndRecordsIt(t *testing.T) {
	dir := useTempConfigDir(t)
	fakeTTY(t, "ttytest")
	profiles := map[string]*config.Profile{
		"builder-104": {Host: "b104", DefaultSession: "work"},
		"aaa":         {Host: "hostA", DefaultSession: "main"},
	}
	saveConfig(t, profiles, "builder-104")

	var gotHost string
	mock := &sshpkg.MockExecutor{}
	orig := newClient
	newClient = func(prof *config.Profile) sshpkg.Executor {
		gotHost = prof.Host
		return mock
	}
	t.Cleanup(func() { newClient = orig })

	// Explicit positional profile "aaa" -> attach hostA:main and record aaa.
	if err := runRoot(rootCmd, []string{"aaa"}); err != nil {
		t.Fatalf("runRoot: %v", err)
	}
	if gotHost != "hostA" {
		t.Errorf("attached host = %q, want hostA", gotHost)
	}
	if len(mock.ExecCalls) != 1 || !strings.Contains(mock.ExecCalls[0], "main") {
		t.Errorf("Exec calls = %v, want attach to session main", mock.ExecCalls)
	}
	if got, _ := config.GetLastProfile(filepath.Join(dir, "lastprofile"), "ttytest"); got != "aaa" {
		t.Errorf("recorded last profile = %q, want aaa", got)
	}
}

// --- killSession ---

func TestKillSession_RunsKillAndCleansAffinity(t *testing.T) {
	dir := useTempConfigDir(t)
	sessDir := filepath.Join(dir, "sessions")
	config.SaveSession(sessDir, "builder-104", "ttys001", "api")
	config.SaveSession(sessDir, "builder-104", "ttys002", "work")

	mock := &sshpkg.MockExecutor{}
	if err := killSession(mock, "builder-104", "myhost", "api"); err != nil {
		t.Fatalf("killSession: %v", err)
	}
	if !mock.LastRunContains("kill-session") || !mock.LastRunContains("api") {
		t.Errorf("expected kill-session for api, got %v", mock.RunCalls)
	}
	if _, err := config.GetSession(sessDir, "builder-104", "ttys001"); err == nil {
		t.Error("affinity record for killed session should be removed")
	}
	if name, _ := config.GetSession(sessDir, "builder-104", "ttys002"); name != "work" {
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
		profile:  "builder-104",
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
	_, err := listSessionsForPicker(mock, "myhost", false)
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("transport error must surface, got %v", err)
	}
}

func TestListSessionsForPicker_NoServerMeansEmpty(t *testing.T) {
	mock := &sshpkg.MockExecutor{
		RunErr:    exitWith(t, 1),
		RunOutput: "no server running on /private/tmp/tmux-501/default",
	}
	items, err := listSessionsForPicker(mock, "myhost", false)
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
	items, err := listSessionsForPicker(mock, "myhost", false)
	if err != nil {
		t.Fatalf("fresh server is not an error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %v, want empty", items)
	}
}

func TestListSessionsForPicker_ParsesSessions(t *testing.T) {
	mock := &sshpkg.MockExecutor{RunOutput: "work: 1 windows\napi: 2 windows"}
	items, err := listSessionsForPicker(mock, "myhost", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
}

// --- profile: default assignment and listing ---

func TestNextDefaultProfile(t *testing.T) {
	cases := []struct {
		prior, name string
		optIn       bool
		want        string
	}{
		{"", "builder-104", false, "builder-104"},            // first profile auto-default
		{"builder-104", "aaa", false, "builder-104"},         // adding another, no opt-in
		{"builder-104", "aaa", true, "aaa"},                  // adding another, opt-in
		{"builder-104", "builder-104", false, "builder-104"}, // editing the default
	}
	for _, c := range cases {
		if got := nextDefaultProfile(c.prior, c.name, c.optIn); got != c.want {
			t.Errorf("nextDefaultProfile(%q,%q,%v) = %q, want %q", c.prior, c.name, c.optIn, got, c.want)
		}
	}
}

func TestWriteProfileList_MarksDefault(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "builder-104",
		Profiles: map[string]*config.Profile{
			"builder-104": {Host: "b104", User: "hien", Port: 22, DefaultSession: "work"},
			"aaa":         {Host: "hostA", User: "root", Port: 2200, DefaultSession: "main"},
		},
	}
	var buf bytes.Buffer
	writeProfileList(&buf, cfg)
	out := buf.String()

	for _, want := range []string{"builder-104", "b104", "aaa", "hostA", "2200", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
	// The default profile's row is marked with "*"; the other is not.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "builder-104") && !strings.HasPrefix(line, "*") {
			t.Errorf("default row not marked with *: %q", line)
		}
		if strings.Contains(line, "aaa") && strings.HasPrefix(line, "*") {
			t.Errorf("non-default row wrongly marked: %q", line)
		}
	}
}

// --- shell: remote exit codes are the user's business ---

func TestRunShell_SwallowsRemoteExitCodes(t *testing.T) {
	useTempConfigDir(t)
	p, def := oneProfile("h", "h")
	saveConfig(t, p, def)
	mock := &sshpkg.MockExecutor{ExecErr: exitWith(t, 1)}
	orig := newClient
	newClient = func(*config.Profile) sshpkg.Executor { return mock }
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
	p, def := oneProfile("h", "h")
	saveConfig(t, p, def)
	mock := &sshpkg.MockExecutor{ExecErr: exec.Command("/nonexistent/ssh").Run()}
	orig := newClient
	newClient = func(*config.Profile) sshpkg.Executor { return mock }
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

// --- local mode ---

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

func TestRootLocalPositionalShorthand(t *testing.T) {
	useTempConfigDir(t)
	mock := &sshpkg.MockExecutor{}
	orig := newLocalClient
	newLocalClient = func() sshpkg.Executor { return mock }
	t.Cleanup(func() { newLocalClient = orig })

	if err := runRoot(rootCmd, []string{"local"}); err != nil {
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

	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \; set-environment WARP_CLIENT_VERSION 'v1' \; set-option -g allow-passthrough on`
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

	if !strings.HasPrefix(mock.ExecCalls[0], `tmux new -A -s 'api' \; `) {
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

func TestWarpRestore_DeletedProfileRemovesRecord(t *testing.T) {
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

func TestWarpRestore_NoRecordLeavesSiblingsAlone(t *testing.T) {
	useTempConfigDir(t)
	forceWarp(t, true, testWarpID)
	dir := warp.LiveDir()
	const sibling = "0000000000000000000000000000dead"
	if err := warp.SaveLive(dir, sibling, "builder-104", "api"); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, sibling), past, past); err != nil {
		t.Fatal(err)
	}
	mock := injectRemoteMock(t)

	captureStdout(t, func() { _ = runWarpRestore(warpRestoreCmd, nil) })

	if len(mock.ExecCalls) != 0 {
		t.Errorf("no record for this tab, yet Exec ran: %v", mock.ExecCalls)
	}
	if _, err := os.Stat(filepath.Join(dir, sibling)); err != nil {
		t.Error("a tab without a record must not prune a sibling's week-old record; that sibling may be restoring right now")
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
