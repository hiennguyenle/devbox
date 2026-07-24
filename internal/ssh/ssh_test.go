package ssh

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// runExit produces a real *exec.ExitError with the given code.
func runExit(t *testing.T, code int) error {
	t.Helper()
	return exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
}

func TestClassifyExit_CleanExit(t *testing.T) {
	action, err := classifyExit(nil)
	if action != actionDone || err != nil {
		t.Errorf("nil error: got (%v, %v), want (actionDone, nil)", action, err)
	}
}

func TestClassifyExit_TransportFailureReconnects(t *testing.T) {
	// ssh reserves 255 for its own failures (connection lost, unreachable)
	action, err := classifyExit(runExit(t, 255))
	if action != actionReconnect || err != nil {
		t.Errorf("exit 255: got (%v, %v), want (actionReconnect, nil)", action, err)
	}
}

func TestClassifyExit_RemoteCommandFailureFails(t *testing.T) {
	// 127 = remote "command not found" (e.g. tmux missing); must NOT reconnect
	action, err := classifyExit(runExit(t, 127))
	if action != actionFail || err == nil {
		t.Errorf("exit 127: got (%v, %v), want (actionFail, non-nil)", action, err)
	}

	// generic remote failure must not reconnect either
	action, err = classifyExit(runExit(t, 1))
	if action != actionFail || err == nil {
		t.Errorf("exit 1: got (%v, %v), want (actionFail, non-nil)", action, err)
	}
}

func TestClassifyExit_CtrlCIsClean(t *testing.T) {
	action, err := classifyExit(runExit(t, 130))
	if action != actionDone || err != nil {
		t.Errorf("exit 130: got (%v, %v), want (actionDone, nil)", action, err)
	}
}

func TestClassifyExit_SpawnFailureFails(t *testing.T) {
	// ssh binary missing locally → *exec.Error, not *exec.ExitError.
	// Must surface the error, never enter the reconnect loop.
	spawnErr := exec.Command("/nonexistent/ssh-binary").Run()
	action, err := classifyExit(spawnErr)
	if action != actionFail || err == nil {
		t.Errorf("spawn failure: got (%v, %v), want (actionFail, non-nil)", action, err)
	}
}

func TestNextAttempt_IncrementsOnQuickDrop(t *testing.T) {
	if got := nextAttempt(5, 2*time.Second); got != 6 {
		t.Errorf("quick drop: got %d, want 6", got)
	}
}

func TestNextAttempt_ResetsAfterStableSession(t *testing.T) {
	// A connection that held for a while means the earlier outage is over;
	// the backoff ladder must restart so the next reconnect is fast again.
	if got := nextAttempt(20, stableSession+time.Second); got != 1 {
		t.Errorf("stable session: got %d, want 1", got)
	}
}

func TestSpinWait_NonTTYTimesOut(t *testing.T) {
	sigCh := make(chan os.Signal)
	if cancelled := spinWait(10*time.Millisecond, "myhost", sigCh, false, nil); cancelled {
		t.Error("expected timeout, got cancelled")
	}
}

func TestSpinWait_NonTTYCancelsOnSignal(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	sigCh <- os.Interrupt
	if cancelled := spinWait(time.Minute, "myhost", sigCh, false, nil); !cancelled {
		t.Error("expected cancellation on signal")
	}
}

func TestProbeReachable_OpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !probeReachable("127.0.0.1", port, time.Second) {
		t.Error("open port should be reachable")
	}
}

func TestProbeReachable_ClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if probeReachable("127.0.0.1", port, 200*time.Millisecond) {
		t.Error("closed port should not be reachable")
	}
}

func TestSpinWait_ReconnectsEarlyWhenNetworkReturns(t *testing.T) {
	// Network was down (first probe fails) then comes back: the wait must
	// end immediately instead of sitting out the full backoff delay.
	origInterval := probeInterval
	probeInterval = 5 * time.Millisecond
	t.Cleanup(func() { probeInterval = origInterval })

	calls := 0
	probe := func() bool {
		calls++
		return calls > 1 // down once, then back up
	}

	sigCh := make(chan os.Signal)
	start := time.Now()
	cancelled := spinWait(10*time.Second, "myhost", sigCh, false, probe)
	elapsed := time.Since(start)

	if cancelled {
		t.Error("early reconnect should not report cancellation")
	}
	if elapsed > time.Second {
		t.Errorf("waited %v; network return should end the wait early", elapsed)
	}
}

func TestSpinWait_NoEarlyExitWithoutDownUpTransition(t *testing.T) {
	// Host reachable the whole time (e.g. ssh failing for auth reasons):
	// never short-circuit the backoff, or we'd hammer the host in a tight loop.
	origInterval := probeInterval
	probeInterval = 5 * time.Millisecond
	t.Cleanup(func() { probeInterval = origInterval })

	probe := func() bool { return true }

	sigCh := make(chan os.Signal)
	delay := 150 * time.Millisecond
	start := time.Now()
	cancelled := spinWait(delay, "myhost", sigCh, false, probe)
	elapsed := time.Since(start)

	if cancelled {
		t.Error("expected timeout, got cancelled")
	}
	if elapsed < delay {
		t.Errorf("waited only %v; always-reachable host must not end the wait early", elapsed)
	}
}

func TestIsTerminal_FalseForPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Error("pipe should not be detected as a terminal")
	}
}

func TestBuildArgs_BasicHost(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104", "tmux list-sessions"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_Interactive(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "tmux new -A -s work")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104", "tmux new -A -s work"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_NoCommand(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_CustomPort(t *testing.T) {
	c := Client{Host: "myhost", User: "admin", Port: 2222}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-p", "2222", "admin@myhost", "tmux list-sessions"}
	assertSliceEqual(t, got, want)
}

func TestReconnectDelay(t *testing.T) {
	if d := reconnectDelay(1); d.Seconds() != 2 {
		t.Errorf("attempt 1: got %v, want 2s", d)
	}
	if d := reconnectDelay(5); d.Seconds() != 5 {
		t.Errorf("attempt 5: got %v, want 5s", d)
	}
	if d := reconnectDelay(8); d.Seconds() != 10 {
		t.Errorf("attempt 8: got %v, want 10s", d)
	}
	if d := reconnectDelay(15); d.Seconds() != 30 {
		t.Errorf("attempt 15: got %v, want 30s", d)
	}
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
