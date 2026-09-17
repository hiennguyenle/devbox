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

// WarpAttachSession is NewAttachSession followed by the configuration a
// Warp tab needs, chained with tmux's `\;` separator so it runs after the
// attach on both the create and attach-existing paths:
//
//   - the session's outward title becomes "session · pane title", so the
//     Warp tab shows the session and whatever Claude Code or Codex wrote
//     into the pane title (session-scoped; other sessions keep tmux.conf);
//   - Warp's version variables are injected into the session environment
//     so Warp's Claude Code plugin in windows opened later can emit
//     notifications (omitted when empty);
//   - allow-passthrough is turned on in memory so those notifications can
//     traverse tmux (tmux >= 3.3; older servers print a status-line error
//     and the attach still succeeds).
func WarpAttachSession(name, protocol, client string) string {
	var b strings.Builder
	b.WriteString(NewAttachSession(name))
	b.WriteString(` \; set-option set-titles on`)
	b.WriteString(` \; set-option set-titles-string '#S · #T'`)
	if protocol != "" {
		b.WriteString(` \; set-environment WARP_CLI_AGENT_PROTOCOL_VERSION ` + quote(protocol))
	}
	if client != "" {
		b.WriteString(` \; set-environment WARP_CLIENT_VERSION ` + quote(client))
	}
	// allow-passthrough is deliberately last: tmux stops the rest of a chained
	// command after an invalid option (probed on 3.6b) but still completes the
	// attach, so on tmux < 3.3, where this option does not exist, nothing else
	// is lost. Do not reorder the steps.
	b.WriteString(` \; set-option -g allow-passthrough on`)
	return b.String()
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
