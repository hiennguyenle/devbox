package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hien/devbox/internal/config"
	sshpkg "github.com/hien/devbox/internal/ssh"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "devbox",
	Short:   "Manage remote tmux sessions over SSH",
	Version: version,
	Args:    cobra.NoArgs,
	RunE:    runRoot,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
}

func Execute() error {
	return rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	sessionName := cfg.DefaultSession

	saveCurrentSession(sessionName)

	remoteCmd := tmux.NewAttachSession(sessionName)
	return handleExecErr(client.Exec(remoteCmd), cfg.Host)
}

func requireConfig() (*config.Config, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("no host configured. Run \"devbox connect\" first")
	}
	return config.LoadDefault()
}

// newClient is a var so tests can inject a MockExecutor.
var newClient = func(cfg *config.Config) sshpkg.Executor {
	return &sshpkg.Client{
		Host:    cfg.Host,
		User:    cfg.User,
		Port:    cfg.Port,
		Verbose: verbose,
	}
}

func saveCurrentSession(name string) {
	tty, err := config.ResolveTTY()
	if err != nil {
		return // silently skip if no TTY
	}
	if err := config.SaveSession(config.SessionsDir(), tty, name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save session state: %v\n", err)
	}
}

// handleSSHError classifies a failed non-interactive Run from its captured
// output ("command not found" only ever appears there, not in err.Error()).
func handleSSHError(output string, err error, host string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(output, "command not found") {
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox connect\"", host)
}

// handleExecErr classifies a failed interactive Exec. Output went straight to
// the terminal, so only the exit code is available: 127 means the remote
// shell couldn't find tmux.
func handleExecErr(err error, host string) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 127 {
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return err
}
