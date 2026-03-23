package cmd

import (
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Will be implemented in Task 8
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print SSH commands being executed")
}

func Execute() error {
	return rootCmd.Execute()
}
