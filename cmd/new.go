package cmd

import (
	"crypto/rand"
	"fmt"

	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new [NAME]",
	Short: "Create or attach to a tmux session (generates a random name if omitted)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	var name string
	if len(args) == 1 {
		name = args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
	} else {
		name = generateSessionID()
	}

	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	client := newClient(cfg)
	fmt.Printf("Creating session %q...\n", name)
	saveCurrentSession(name)

	return client.Exec(tmux.NewAttachSession(name))
}

func generateSessionID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("s-%x", b)
}
