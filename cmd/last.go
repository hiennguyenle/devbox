package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/config"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var lastCmd = &cobra.Command{
	Use:   "last",
	Short: "Resume the most recently used session (global)",
	RunE:  runLast,
}

func init() {
	rootCmd.AddCommand(lastCmd)
}

func runLast(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)

	sessionName, err := config.LastSession(config.SessionsDir())
	if err != nil {
		fmt.Println("No session history. Launching session picker...")
		return runAttachPicker(cfg, client)
	}

	// Verify session still exists
	_, checkErr := client.Run(tmux.HasSession(sessionName))
	if checkErr != nil {
		fmt.Printf("Session %q no longer exists. Launching session picker...\n", sessionName)
		return runAttachPicker(cfg, client)
	}

	fmt.Printf("Resuming %q...\n", sessionName)
	saveCurrentSession(sessionName)
	return client.Exec(tmux.NewAttachSession(sessionName))
}
