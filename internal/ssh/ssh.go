package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
)

type Client struct {
	Host    string
	User    string
	Port    int
	Verbose bool
}

func (c *Client) buildArgs(interactive bool, remoteCmd string) []string {
	var args []string
	if interactive {
		args = append(args, "-t")
	}
	args = append(args, "-o", "ConnectTimeout=10")
	args = append(args, "-o", "ServerAliveInterval=30")
	args = append(args, "-o", "ServerAliveCountMax=3")
	args = append(args, "-p", fmt.Sprintf("%d", c.Port))
	args = append(args, fmt.Sprintf("%s@%s", c.User, c.Host))
	if remoteCmd != "" {
		args = append(args, remoteCmd)
	}
	return args
}

func (c *Client) Run(remoteCmd string) (string, error) {
	args := c.buildArgs(false, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Exec runs an interactive SSH session with auto-reconnect.
// On connection failure, it retries with backoff until the user presses Ctrl+C.
func (c *Client) Exec(remoteCmd string) error {
	args := c.buildArgs(true, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}

	// Catch Ctrl+C during reconnect wait
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	attempt := 0
	for {
		cmd := exec.Command("ssh", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()

		// Clean exit (user detached from tmux or exited shell)
		if err == nil {
			return nil
		}

		// Check if user intentionally exited (exit code 0 means clean)
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			// Exit code 0 = clean exit, don't reconnect
			// Exit code 130 = Ctrl+C, don't reconnect
			if code == 0 || code == 130 {
				return nil
			}
		}

		// Connection failure — auto-reconnect
		attempt++
		delay := reconnectDelay(attempt)

		fmt.Fprintf(os.Stderr, "\nConnection lost. Reconnecting in %s... (Ctrl+C to cancel)\n", delay)

		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "\nReconnect cancelled.")
			return nil
		case <-time.After(delay):
			fmt.Fprintf(os.Stderr, "Reconnecting to %s (attempt %d)...\n", c.Host, attempt)
		}
	}
}

func reconnectDelay(attempt int) time.Duration {
	switch {
	case attempt <= 3:
		return 2 * time.Second
	case attempt <= 6:
		return 5 * time.Second
	case attempt <= 10:
		return 10 * time.Second
	default:
		return 30 * time.Second
	}
}
