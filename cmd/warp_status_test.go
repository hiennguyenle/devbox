package cmd

import (
	"bytes"
	"testing"
)

func TestWarpStatusCommandIsRegistered(t *testing.T) {
	command, _, err := warpCmd.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find warp status command: %v", err)
	}
	if command == warpCmd {
		t.Fatal("warp status command is not registered")
	}
}

func TestWarpStatusCommandPrintsTmuxConfig(t *testing.T) {
	var out bytes.Buffer
	warpStatusCmd.SetOut(&out)
	if err := runWarpStatus(warpStatusCmd, nil); err != nil {
		t.Fatalf("run warp status: %v", err)
	}
	const want = "Add this to ~/.tmux.conf on the managed host:\n" +
		"set -g status-left \"#[fg=black,bg=green,bold] #{?#{m/i:*codex*,#{pane_current_command}},🤖 Codex,#{?#{m/i:*codex*,#{window_name}},🤖 Codex,#{?#{m/i:*claude*,#{pane_current_command}},✦ Claude,#{?#{m/i:*claude*,#{window_name}},✦ Claude,⌘ #W}}}} · #H · #S #[default] \"\n"
	if got := out.String(); got != want {
		t.Errorf("warp status output = %q, want %q", got, want)
	}
}
