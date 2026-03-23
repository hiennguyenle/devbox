package cmd

import (
	"fmt"

	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill NAME",
	Short: "Kill a tmux session by name",
	Args:  cobra.ExactArgs(1),
	RunE:  runKill,
}

func init() {
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validate.SessionName(name); err != nil {
		return err
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	output, err := client.Run(tmux.KillSession(name))
	if err != nil {
		return fmt.Errorf("kill session %q: %s", name, output)
	}

	fmt.Printf("Session %q killed.\n", name)
	return nil
}
