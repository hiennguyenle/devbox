// Package warp integrates devbox with the Warp terminal: it detects Warp,
// generates Warp tab configs, tracks which Warp tab is attached to which
// session, and prints the shell hook that reattaches restored tabs.
package warp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

// Detect reports whether devbox is running inside the Warp terminal. A var
// so tests can force either branch without touching the environment.
var Detect = func() bool {
	return os.Getenv("TERM_PROGRAM") == "WarpTerminal"
}

var sessionIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// SessionID returns the Warp session UUID of this tab, or "" when unset or
// malformed. Warp keeps the UUID stable across restarts and reboots, which
// is what lets a restored tab find its own live record. A var so tests can
// inject one.
var SessionID = func() string {
	id := os.Getenv("WARP_TERMINAL_SESSION_UUID")
	if !sessionIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// envValuePattern keeps the values devbox forwards into a remote tmux
// command to a single safe word (they are single-quoted anyway; this is
// defence in depth).
var envValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Env returns Warp's CLI-agent protocol version and client version, each ""
// when unset or invalid. They are forwarded into the remote tmux session so
// Warp's Claude Code plugin there sees the same values a local shell would.
func Env() (protocol, client string) {
	protocol = os.Getenv("WARP_CLI_AGENT_PROTOCOL_VERSION")
	if !envValuePattern.MatchString(protocol) {
		protocol = ""
	}
	client = os.Getenv("WARP_CLIENT_VERSION")
	if !envValuePattern.MatchString(client) {
		client = ""
	}
	return protocol, client
}

// TabConfigsDir returns Warp's tab_configs directory for this platform. It
// never creates the directory: a missing one means Warp is not installed
// (or has never run), and that is an error the user should see.
func TabConfigsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	var dir string
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(home, ".warp", "tab_configs")
	} else {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "warp-terminal", "tab_configs")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("warp tab configs directory %s: %w", dir, err)
		}
		return "", fmt.Errorf("warp tab configs directory not found: %s", dir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("warp tab configs directory not found: %s", dir)
	}
	return dir, nil
}
