package ssh

import "testing"

func TestBuildArgs_BasicHost(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104", "tmux list-sessions"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_Interactive(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "tmux new -A -s work")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104", "tmux new -A -s work"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_NoCommand(t *testing.T) {
	c := Client{Host: "builder-104", User: "hien", Port: 22}
	got := c.buildArgs(true, "")
	want := []string{"-t", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-o", "ServerAliveCountMax=3", "-p", "22", "hien@builder-104"}
	assertSliceEqual(t, got, want)
}

func TestBuildArgs_CustomPort(t *testing.T) {
	c := Client{Host: "myhost", User: "admin", Port: 2222}
	got := c.buildArgs(false, "tmux list-sessions")
	want := []string{"-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-o", "ServerAliveCountMax=3", "-p", "2222", "admin@myhost", "tmux list-sessions"}
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
