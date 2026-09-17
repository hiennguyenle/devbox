package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/hiennguyenle/devbox/internal/config"
	"github.com/hiennguyenle/devbox/internal/warp"
	"github.com/spf13/cobra"
)

var warpCmd = &cobra.Command{
	Use:   "warp",
	Short: "Warp terminal integration",
}

var warpSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Generate Warp tab configs for every profile",
	Args:  cobra.NoArgs,
	RunE:  runWarpSync,
}

var warpHookCmd = &cobra.Command{
	Use:   "hook SHELL",
	Short: "Print the shell snippet that reattaches restored Warp tabs (zsh, bash)",
	Args:  cobra.ExactArgs(1),
	RunE:  runWarpHook,
}

func init() {
	warpCmd.AddCommand(warpSyncCmd, warpHookCmd)
	rootCmd.AddCommand(warpCmd)
}

// warpTabConfigsDir is a var so tests can point sync at a temp dir.
var warpTabConfigsDir = warp.TabConfigsDir

func runWarpSync(cmd *cobra.Command, args []string) error {
	dir, err := warpTabConfigsDir()
	if err != nil {
		return err
	}
	var cfg *config.Config
	if config.Exists() {
		cfg, err = config.LoadDefault()
		if err != nil {
			return err
		}
	}
	changes, err := warp.Sync(dir, cfg)
	printWarpChanges(os.Stdout, changes)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("To reattach devbox tabs after a reboot, add to ~/.zshrc:")
	fmt.Println(`  eval "$(devbox warp hook zsh)"`)
	return nil
}

// printWarpChanges renders one line per file: "<action> <path>".
func printWarpChanges(out io.Writer, changes []warp.Change) {
	for _, c := range changes {
		line := c.Action + " " + c.Path
		if c.Action == warp.ActionSkipped {
			line += " (not managed by devbox)"
		}
		fmt.Fprintln(out, line)
	}
}

func runWarpHook(cmd *cobra.Command, args []string) error {
	snippet, err := warp.Hook(args[0])
	if err != nil {
		return err
	}
	fmt.Print(snippet)
	return nil
}
