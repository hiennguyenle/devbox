package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
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
	args = append(args, "-o", "ServerAliveInterval=10")
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

type exitAction int

const (
	actionDone      exitAction = iota // clean exit: return nil
	actionReconnect                   // transport failure: retry with backoff
	actionFail                        // real error: surface it, never retry
)

// classifyExit decides what Exec does after ssh exits. ssh reserves exit
// code 255 for its own failures (connection lost, host unreachable); every
// other code is the remote command's exit status and must not trigger a
// reconnect.
func classifyExit(err error) (exitAction, error) {
	if err == nil {
		return actionDone, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return actionFail, fmt.Errorf("cannot run ssh: %w", err)
	}
	switch exitErr.ExitCode() {
	case 130: // Ctrl+C
		return actionDone, nil
	case -1: // killed by a local signal — user-driven, don't respin
		return actionDone, nil
	case 255:
		return actionReconnect, nil
	default:
		return actionFail, err
	}
}

func classifyExitWithDisconnect(err error, onDisconnect func(), notified *bool) (exitAction, error) {
	action, classifiedErr := classifyExit(err)
	if action == actionReconnect && !*notified {
		if onDisconnect != nil {
			onDisconnect()
		}
		*notified = true
	}
	return action, classifiedErr
}

// stableSession is how long a connection must hold before a subsequent drop
// restarts the backoff ladder instead of continuing where it left off.
const stableSession = 30 * time.Second

func nextAttempt(prev int, connected time.Duration) int {
	if connected >= stableSession {
		return 1
	}
	return prev + 1
}

// Exec runs an interactive SSH session with auto-reconnect. Reconnection is
// intentionally unbounded: the whole point is to survive network roaming
// (office → home) and reattach whenever connectivity returns. Only transport
// failures retry; remote command errors surface immediately, and Ctrl+C
// during the wait cancels.
func (c *Client) Exec(remoteCmd string, onDisconnect func()) error {
	args := c.buildArgs(true, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}

	// Catch Ctrl+C during reconnect wait
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	tty := isTerminal(os.Stderr)
	probe := func() bool { return probeReachable(c.Host, c.Port, time.Second) }
	attempt := 0
	disconnectNotified := false
	for {
		cmd := exec.Command("ssh", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		start := time.Now()
		runErr := cmd.Run()

		action, err := classifyExitWithDisconnect(runErr, onDisconnect, &disconnectNotified)
		if action != actionReconnect {
			return err
		}

		attempt = nextAttempt(attempt, time.Since(start))
		delay := reconnectDelay(attempt)

		if spinWait(delay, c.Host, sigCh, tty, probe) {
			if tty {
				fmt.Fprintf(os.Stderr, "\r\033[K")
			}
			return nil
		}
	}
}

// probeInterval is how often spinWait checks whether the host's SSH port is
// reachable again. A var so tests can shrink it.
var probeInterval = 2 * time.Second

// probeReachable reports whether a TCP connection to host:port succeeds
// within timeout.
func probeReachable(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// watchNetwork probes until the network transitions from down to up, then
// signals upCh. Requiring an observed DOWN first means an ssh failure on a
// reachable host (auth, host key) still backs off normally instead of
// hammering the server in a tight loop.
func watchNetwork(probe func() bool, upCh chan<- struct{}, stop <-chan struct{}) {
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	sawDown := false
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if !probe() {
				sawDown = true
				continue
			}
			if sawDown {
				select {
				case upCh <- struct{}{}:
				case <-stop:
				}
				return
			}
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

// isTerminal reports whether f is a character device, so ANSI spinner codes
// only reach real terminals.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// spinWait waits out the reconnect delay, showing an animated spinner on
// terminals or a single plain line otherwise. It ends early when probe
// observes the network coming back (down → up), so reattach is immediate
// instead of sitting out the full backoff. Returns true if cancelled by
// Ctrl+C.
func spinWait(delay time.Duration, host string, sigCh <-chan os.Signal, tty bool, probe func() bool) bool {
	upCh := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)
	if probe != nil {
		go watchNetwork(probe, upCh, stop)
	}

	if !tty {
		fmt.Fprintf(os.Stderr, "Reconnecting to %s...\n", host)
		select {
		case <-sigCh:
			return true
		case <-upCh:
			return false
		case <-time.After(delay):
			return false
		}
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(delay)
	frame := 0

	for {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s Reconnecting to %s...", frames[frame%len(frames)], host)
		frame++

		select {
		case <-sigCh:
			return true
		case <-upCh:
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return false
		case <-deadline:
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return false
		case <-ticker.C:
			// next frame
		}
	}
}
