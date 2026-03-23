package cmd

import (
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

	client := newClient(cfg)
	return client.Exec("")
}
