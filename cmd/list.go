package cmd

import (
	"fmt"
	"strings"

	"github.com/hien/devbox/internal/tmux"
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
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if strings.Contains(output, "no server running") || strings.Contains(output, "no sessions") {
			fmt.Printf("No active sessions on %s.\n", cfg.Host)
			return nil
		}
		return handleSSHError(err, cfg.Host)
	}

	fmt.Println(output)
	return nil
}
