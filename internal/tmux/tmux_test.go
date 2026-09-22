package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	want := `tmux new -A -s 'api' -e 'WARP_CLI_AGENT_PROTOCOL_VERSION=1' -e 'WARP_CLIENT_VERSION=v0.2026.09.02.08.27.stable_01' \; set-option set-titles on \; set-option set-titles-string '#{?#{m/i:*codex*,#{pane_current_command}},🤖 Codex,#{?#{m/i:*codex*,#{window_name}},🤖 Codex,#{?#{m/i:*claude*,#{pane_current_command}},✦ Claude,#{?#{m/i:*claude*,#{window_name}},✦ Claude,⌘ #W}}}} · #S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \; set-environment WARP_CLIENT_VERSION 'v0.2026.09.02.08.27.stable_01' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_OmitsEmptyEnvSteps(t *testing.T) {
	got := WarpAttachSession("api", "", "")
	want := `tmux new -A -s 'api' \; set-option set-titles on \; set-option set-titles-string '#{?#{m/i:*codex*,#{pane_current_command}},🤖 Codex,#{?#{m/i:*codex*,#{window_name}},🤖 Codex,#{?#{m/i:*claude*,#{pane_current_command}},✦ Claude,#{?#{m/i:*claude*,#{window_name}},✦ Claude,⌘ #W}}}} · #S · #T' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_OmitsClientStepWhenOnlyProtocolIsSet(t *testing.T) {
	got := WarpAttachSession("api", "1", "")
	want := `tmux new -A -s 'api' -e 'WARP_CLI_AGENT_PROTOCOL_VERSION=1' \; set-option set-titles on \; set-option set-titles-string '#{?#{m/i:*codex*,#{pane_current_command}},🤖 Codex,#{?#{m/i:*codex*,#{window_name}},🤖 Codex,#{?#{m/i:*claude*,#{pane_current_command}},✦ Claude,#{?#{m/i:*claude*,#{window_name}},✦ Claude,⌘ #W}}}} · #S · #T' \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION '1' \; set-option -g allow-passthrough on`
	if got != want {
		t.Errorf("WarpAttachSession =\n%s\nwant\n%s", got, want)
	}
}

func TestWarpAttachSession_OmitsProtocolStepWhenOnlyClientIsSet(t *testing.T) {
	got := WarpAttachSession("api", "", "v1")
	want := `tmux new -A -s 'api' -e 'WARP_CLIENT_VERSION=v1' \; set-option set-titles on \; set-option set-titles-string '#{?#{m/i:*codex*,#{pane_current_command}},🤖 Codex,#{?#{m/i:*codex*,#{window_name}},🤖 Codex,#{?#{m/i:*claude*,#{pane_current_command}},✦ Claude,#{?#{m/i:*claude*,#{window_name}},✦ Claude,⌘ #W}}}} · #S · #T' \; set-environment WARP_CLIENT_VERSION 'v1' \; set-option -g allow-passthrough on`
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

func TestAgentMarkerUsesCaseInsensitiveGlob(t *testing.T) {
	if strings.Contains(agentMarker, "m/ri:") {
		t.Fatalf("agent marker uses regex mode with glob patterns: %q", agentMarker)
	}
	if !strings.Contains(agentMarker, "m/i:*codex*") || !strings.Contains(agentMarker, "m/i:*claude*") {
		t.Fatalf("agent marker is missing case-insensitive Codex/Claude glob patterns: %q", agentMarker)
	}
}

func TestWarpAttachSession_FirstShellEnvironment(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	// A short path keeps the Unix socket below macOS's length limit.
	dir, err := os.MkdirTemp("", "dbw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() { exec.Command(bin, "-S", socket, "kill-server").Run() })
	t.Setenv("TMUX", "")
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "")
	t.Setenv("WARP_CLIENT_VERSION", "")

	result := filepath.Join(dir, "env")
	shell := `printf '%s\n%s\n' "$WARP_CLI_AGENT_PROTOCOL_VERSION" "$WARP_CLIENT_VERSION" > ` + quote(result) + `; exec sleep 30`
	command := WarpAttachSession("env-test", "1", "v-test")
	command = strings.Replace(command, "tmux new -A", quote(bin)+" -S "+quote(socket)+" -f /dev/null new -A -d", 1)
	command = strings.Replace(command, ` \;`, " "+quote("/bin/sh -c "+quote(shell))+` \;`, 1)
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("isolated tmux: %v\n%s", err, out)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(result)
		if err == nil && string(data) == "1\nv-test\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first shell environment = %q, err = %v", data, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for key, want := range map[string]string{"WARP_CLI_AGENT_PROTOCOL_VERSION": "1", "WARP_CLIENT_VERSION": "v-test"} {
		out, err := exec.Command(bin, "-S", socket, "show-environment", "-t", "=env-test", key).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != key+"="+want {
			t.Errorf("session environment %s = %q, err = %v", key, out, err)
		}
	}
}
