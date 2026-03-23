package cmd

import (
	"fmt"
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
	return client.Exec(remoteCmd)
}

func requireConfig() (*config.Config, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("no host configured. Run \"devbox connect\" first")
	}
	return config.LoadDefault()
}

func newClient(cfg *config.Config) *sshpkg.Client {
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
	config.SaveSession(config.SessionsDir(), tty, name)
}

func handleSSHError(err error, host string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "command not found") {
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox connect\"", host)
}
