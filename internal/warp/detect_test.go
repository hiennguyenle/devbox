package warp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "WarpTerminal")
	if !Detect() {
		t.Error("Detect() = false inside Warp, want true")
	}
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if Detect() {
		t.Error("Detect() = true outside Warp, want false")
	}
}

func TestSessionID_AcceptsWarpUUID(t *testing.T) {
	t.Setenv("WARP_TERMINAL_SESSION_UUID", "aca3a00ac45440f19afea15ac826acd0")
	if got := SessionID(); got != "aca3a00ac45440f19afea15ac826acd0" {
		t.Errorf("SessionID() = %q, want the env value", got)
	}
}

func TestSessionID_RejectsMalformed(t *testing.T) {
	bad := []string{"", "ACA3A00AC45440F19AFEA15AC826ACD0", "../etc/passwd", "abc", "aca3a00ac45440f19afea15ac826acd0x"}
	for _, v := range bad {
		t.Setenv("WARP_TERMINAL_SESSION_UUID", v)
		if got := SessionID(); got != "" {
			t.Errorf("SessionID() with %q = %q, want empty", v, got)
		}
	}
}

func TestEnv_ReadsAndValidates(t *testing.T) {
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.09.02.08.27.stable_01")
	p, c := Env()
	if p != "1" || c != "v0.2026.09.02.08.27.stable_01" {
		t.Errorf("Env() = %q, %q; want 1, v0.2026.09.02.08.27.stable_01", p, c)
	}
}

func TestEnv_RequiresAlphanumericFirstCharacter(t *testing.T) {
	// A leading "-" would turn the value into a flag for the remote command.
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "-1")
	t.Setenv("WARP_CLIENT_VERSION", "v2024.01-beta_1")
	if p, c := Env(); p != "" || c != "v2024.01-beta_1" {
		t.Errorf("Env() = %q, %q; want empty protocol and v2024.01-beta_1", p, c)
	}
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1.2.3")
	if p, _ := Env(); p != "1.2.3" {
		t.Errorf("Env() protocol = %q, want 1.2.3", p)
	}
}

func TestEnv_RejectsShellMetacharacters(t *testing.T) {
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1; rm -rf /")
	t.Setenv("WARP_CLIENT_VERSION", "")
	p, c := Env()
	if p != "" || c != "" {
		t.Errorf("Env() = %q, %q; want both empty", p, c)
	}
}

func TestTabConfigsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	want := filepath.Join(home, ".warp", "tab_configs")
	if runtime.GOOS != "darwin" {
		want = filepath.Join(home, ".local", "share", "warp-terminal", "tab_configs")
	}

	if _, err := TabConfigsDir(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("missing dir: err = %v, want error naming %s", err, want)
	}
	if err := os.MkdirAll(want, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := TabConfigsDir()
	if err != nil || got != want {
		t.Errorf("TabConfigsDir() = %q, %v; want %q", got, err, want)
	}
}
