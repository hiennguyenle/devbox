package tmux

import "testing"

func TestNewAttachSession(t *testing.T) {
	got := NewAttachSession("work")
	want := "tmux new -A -s 'work'"
	if got != want {
		t.Errorf("NewAttachSession(work) = %q, want %q", got, want)
	}
}

func TestNewAttachSession_QuotesMetacharacters(t *testing.T) {
	// Session names sourced from remote list-sessions can contain anything
	// tmux allows; the remote shell must see them as one literal word.
	got := NewAttachSession("a b;$(rm x)")
	want := `tmux new -A -s 'a b;$(rm x)'`
	if got != want {
		t.Errorf("NewAttachSession = %q, want %q", got, want)
	}
}

func TestNewAttachSession_EscapesSingleQuotes(t *testing.T) {
	got := NewAttachSession("it's")
	want := `tmux new -A -s 'it'\''s'`
	if got != want {
		t.Errorf("NewAttachSession = %q, want %q", got, want)
	}
}

func TestListSessions(t *testing.T) {
	got := ListSessions()
	want := "tmux list-sessions"
	if got != want {
		t.Errorf("ListSessions() = %q, want %q", got, want)
	}
}

func TestKillSession_ExactMatchTarget(t *testing.T) {
	// "=" prefix forces exact match so "api" can never prefix-match "api2"
	got := KillSession("api")
	want := "tmux kill-session -t '=api'"
	if got != want {
		t.Errorf("KillSession(api) = %q, want %q", got, want)
	}
}

func TestRenameSession_ExactMatchTarget(t *testing.T) {
	got := RenameSession("old", "new")
	want := "tmux rename-session -t '=old' 'new'"
	if got != want {
		t.Errorf("RenameSession(old, new) = %q, want %q", got, want)
	}
}
