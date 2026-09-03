package cmd

import (
	"fmt"

	"github.com/hiennguyenle/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tmux sessions on the remote host",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	_, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if isNoServerOutput(output) {
			fmt.Printf("No active sessions on %s.\n", host)
			return nil
		}
		return handleSSHError(output, err, host, localFlag)
	}

	fmt.Println(output)
	return nil
}
