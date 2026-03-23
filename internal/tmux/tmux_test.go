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
