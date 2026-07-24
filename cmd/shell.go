package cmd

import (
	"errors"
	"os/exec"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a raw SSH shell (no tmux)",
	RunE:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	_, prof, err := resolveProfile(cfg, "")
	if err != nil {
		return err
	}

	client := newClient(prof)
	err = client.Exec("")
	// In a raw shell the exit status belongs to the user (`exit 1` is not a
	// devbox failure); only surface non-exit errors like a missing ssh binary.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}
