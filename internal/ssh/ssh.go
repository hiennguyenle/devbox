package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
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

func (c *Client) Exec(remoteCmd string) error {
	args := c.buildArgs(true, remoteCmd)
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "ssh %s\n", strings.Join(args, " "))
	}
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}
	argv := append([]string{"ssh"}, args...)
	return syscall.Exec(sshPath, argv, os.Environ())
}
