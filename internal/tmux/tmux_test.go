package tmux

import (
	"strings"
	"testing"
)

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

func TestWarpAttachSession_FullChain(t *testing.T) {
	got := WarpAttachSession("api", "1", "v0.2026.09.02.08.27.stable_01")
	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \; set-environment WARP_CLIENT_VERSION 'v0.2026.09.02.08.27.stable_01' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_OmitsEmptyEnvSteps(t *testing.T) {
	got := WarpAttachSession("api", "", "")
	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#S · #T' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_QuotesSessionName(t *testing.T) {
	got := WarpAttachSession("it's", "", "")
	if want := `tmux new -A -s 'it'\''s' \; `; !strings.HasPrefix(got, want) {
		t.Errorf("WarpAttachSession = %q, want prefix %q", got, want)
	}
}
