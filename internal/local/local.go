// Package local runs devbox's command strings on this machine instead of a
// remote host. Executor implements ssh.Executor so commands stay
// transport-agnostic.
package local

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Executor struct {
	Verbose bool
}

// Run executes a command through sh and captures its output, mirroring
// ssh.Client.Run.
func (e *Executor) Run(command string) (string, error) {
	if e.Verbose {
		fmt.Fprintf(os.Stderr, "local %s\n", command)
	}
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Exec runs a command with the terminal attached. An empty command opens the
// user's shell, mirroring `devbox shell`. onDisconnect is accepted for
// interface parity and never called: there is no transport to lose.
func (e *Executor) Exec(command string, onDisconnect func()) error {
	var cmd *exec.Cmd
	if command == "" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = exec.Command(shell)
		if e.Verbose {
			fmt.Fprintf(os.Stderr, "local %s\n", shell)
		}
	} else {
		cmd = exec.Command("sh", "-c", command)
		if e.Verbose {
			fmt.Fprintf(os.Stderr, "local %s\n", command)
		}
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
