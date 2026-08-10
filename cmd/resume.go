package cmd

import (
	"github.com/hiennguyenle/devbox/internal/config"
	"github.com/hiennguyenle/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [NAME]",
	Short: "Attach to a session (interactive picker if no name given)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	profName, prof, err := resolveProfile(cfg, "")
	if err != nil {
		return err
	}
	client := newClient(prof)

	// Named resume — direct attach
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		recordProfileSession(profName, name)
		return execTmuxSession(client, prof.Host, name)
	}

	// No name — interactive picker, pre-select terminal's last session on this profile
	preSelect := ""
	if tty, err := config.ResolveTTY(); err == nil {
		if name, err := config.GetSession(config.SessionsDir(), profName, tty); err == nil {
			preSelect = name
		}
	}

	return runAttachPicker(profName, prof, client, preSelect)
}
