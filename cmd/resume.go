package cmd

import (
	"github.com/hien/devbox/internal/config"
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
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

	client := newClient(cfg)

	// Named resume — direct attach
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		saveCurrentSession(name)
		return handleExecErr(client.Exec(tmux.NewAttachSession(name)), cfg.Host)
	}

	// No name — interactive picker, pre-select terminal's last session
	preSelect := ""
	tty, err := config.ResolveTTY()
	if err == nil {
		if name, err := config.GetSession(config.SessionsDir(), tty); err == nil {
			preSelect = name
		}
	}

	return runAttachPicker(cfg, client, preSelect)
}
