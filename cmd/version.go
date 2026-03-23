package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print devbox version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("devbox version %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
