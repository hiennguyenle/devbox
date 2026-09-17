package cmd

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/hiennguyenle/devbox/internal/validate"
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

	profName, host, _, client, err := resolveExecutor("")
	if err != nil {
		return err
	}

	fmt.Printf("Creating session %q...\n", name)
	recordProfileSession(profName, name)

	return execTmuxSession(client, profName, host, name)
}

func generateSessionID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; fall back to a time-derived id
		// rather than the all-zero buffer
		binary.LittleEndian.PutUint32(b, uint32(time.Now().UnixNano()))
	}
	return fmt.Sprintf("s-%x", b)
}
