package cmd

import (
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new NAME",
	Short: "Create or attach to a named tmux session",
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validate.SessionName(name); err != nil {
		return err
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	saveCurrentSession(name)

	return client.Exec(tmux.NewAttachSession(name))
}
