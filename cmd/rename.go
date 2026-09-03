package cmd

import (
	"fmt"

	"github.com/hiennguyenle/devbox/internal/tmux"
	"github.com/hiennguyenle/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename OLD NEW",
	Short: "Rename a tmux session",
	Args:  cobra.ExactArgs(2),
	RunE:  runRename,
}

func init() {
	rootCmd.AddCommand(renameCmd)
}

func runRename(cmd *cobra.Command, args []string) error {
	oldName, newName := args[0], args[1]
	if err := validate.SessionName(oldName); err != nil {
		return fmt.Errorf("old name: %w", err)
	}
	if err := validate.SessionName(newName); err != nil {
		return fmt.Errorf("new name: %w", err)
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	_, prof, err := resolveProfile(cfg, "")
	if err != nil {
		return err
	}

	client := newClient(prof)
	output, err := client.Run(tmux.RenameSession(oldName, newName))
	if err != nil {
		if output != "" {
			return fmt.Errorf("rename session: %s", output)
		}
		return handleSSHError(output, err, prof.Host, false)
	}

	fmt.Printf("Session %q renamed to %q.\n", oldName, newName)
	return nil
}
