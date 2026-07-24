package ssh

// Executor abstracts SSH command execution for testability.
// Commands pass remote command strings (e.g. "tmux list-sessions");
// the implementation handles SSH arg construction internally.
type Executor interface {
	// Run executes a non-interactive SSH command and captures output.
	Run(remoteCmd string) (string, error)

	// Exec runs an interactive SSH session in the foreground, auto-reconnecting
	// on transport failure until the session ends cleanly.
	Exec(remoteCmd string) error
}
