package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hiennguyenle/devbox/internal/config"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
	"github.com/hiennguyenle/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var (
	verbose     bool
	profileFlag string
	version     = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "devbox [PROFILE]",
	Short:   "Manage remote tmux sessions over SSH",
	Version: version,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runRoot,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
	rootCmd.PersistentFlags().StringVarP(&profileFlag, "profile", "p", "", "profile to act on (overrides last-used/default)")
}

func Execute() error {
	return rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	explicit := ""
	if len(args) == 1 {
		explicit = args[0]
	}
	name, prof, err := resolveProfile(cfg, explicit)
	if err != nil {
		return err
	}

	client := newClient(prof)
	recordProfileSession(name, prof.DefaultSession)

	remoteCmd := tmux.NewAttachSession(prof.DefaultSession)
	return handleExecErr(client.Exec(remoteCmd), prof.Host)
}

func requireConfig() (*config.Config, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("no profiles configured. Run \"devbox profile\" first")
	}
	return config.LoadDefault()
}

// newClient is a var so tests can inject a MockExecutor.
var newClient = func(prof *config.Profile) sshpkg.Executor {
	return &sshpkg.Client{
		Host:    prof.Host,
		User:    prof.User,
		Port:    prof.Port,
		Verbose: verbose,
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
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox profile\"", host)
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
