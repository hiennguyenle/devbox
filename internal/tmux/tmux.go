package tmux

import (
	"fmt"
	"strings"
)

// quote wraps s in single quotes for the remote shell, so session names
// (including ones created out-of-band with spaces or metacharacters) are
// always a single literal word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// target builds an exact-match tmux target ("=name"), preventing tmux's
// prefix matching from resolving "api" to "api2".
func target(name string) string {
	return quote("=" + name)
}

func NewAttachSession(name string) string {
	return fmt.Sprintf("tmux new -A -s %s", quote(name))
}

func ListSessions() string {
	return "tmux list-sessions"
}

func KillSession(name string) string {
	return fmt.Sprintf("tmux kill-session -t %s", target(name))
}

func RenameSession(oldName, newName string) string {
	return fmt.Sprintf("tmux rename-session -t %s %s", target(oldName), quote(newName))
}
