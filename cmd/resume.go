package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/config"
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [NAME]",
	Short: "Attach to a session (by name, or resume this terminal's last session)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)

	// Named resume
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		saveCurrentSession(name)
		return client.Exec(tmux.NewAttachSession(name))
	}

	// Per-terminal resume
	tty, err := config.ResolveTTY()
	if err != nil {
		fmt.Println("Cannot determine terminal. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	sessionName, err := config.GetSession(config.SessionsDir(), tty)
	if err != nil {
		fmt.Println("No session for this terminal. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	// Verify session still exists on remote
	_, checkErr := client.Run(tmux.HasSession(sessionName))
	if checkErr != nil {
		config.DeleteSession(config.SessionsDir(), tty)
		fmt.Printf("Session %q no longer exists. Launching session picker...\n", sessionName)
		return runAttachPicker(cfg, client)
	}

	saveCurrentSession(sessionName)
	return client.Exec(tmux.NewAttachSession(sessionName))
}
