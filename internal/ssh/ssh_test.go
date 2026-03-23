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
