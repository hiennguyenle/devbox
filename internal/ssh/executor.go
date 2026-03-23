package ssh

// Executor abstracts SSH command execution for testability.
// Commands pass remote command strings (e.g. "tmux list-sessions");
// the implementation handles SSH arg construction internally.
type Executor interface {
	// Run executes a non-interactive SSH command and captures output.
	Run(remoteCmd string) (string, error)

	// Exec replaces the current process with SSH (interactive mode).
	Exec(remoteCmd string) error
}
