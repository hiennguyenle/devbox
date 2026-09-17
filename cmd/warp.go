package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/hiennguyenle/devbox/internal/config"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
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

var warpRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Reattach this Warp tab's session after a restart (run by the shell hook)",
	Args:  cobra.NoArgs,
	RunE:  runWarpRestore,
	// Runs unattended from a shell hook: an attach error (host still down
	// right after a reboot) should print the error, not the usage block.
	SilenceUsage: true,
}

func init() {
	warpCmd.AddCommand(warpSyncCmd, warpHookCmd, warpRestoreCmd)
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

// liveRecordMaxAge is how long a live record may sit unused before it is
// treated as belonging to a tab that will never come back.
const liveRecordMaxAge = 7 * 24 * time.Hour

// runWarpRestore is what the shell hook runs at every interactive shell
// start inside Warp. It is silent unless this exact Warp tab was attached
// through devbox when the process died, in which case it reattaches.
func runWarpRestore(cmd *cobra.Command, args []string) error {
	// A shell inside the tmux session devbox just attached is not a restored
	// tab. tmux >= 3.3 rewrites TERM_PROGRAM so the hook never fires there;
	// the TMUX check covers older servers, where a nested restore would fail
	// the attach and then clear the outer tab's record.
	if !warp.Detect() || os.Getenv("TMUX") != "" {
		return nil
	}
	id := warp.SessionID()
	if id == "" {
		return nil
	}
	dir := warp.LiveDir()

	profName, session, err := warp.LoadLive(dir, id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		warp.ClearLive(dir, id)
		fmt.Fprintf(os.Stderr, "warning: removed unreadable warp live record: %v\n", err)
		return nil
	}

	var host string
	var client sshpkg.Executor
	if profName == localProfileName {
		host, client = "localhost", newLocalClient()
	} else {
		if !config.Exists() {
			warp.ClearLive(dir, id)
			fmt.Fprintf(os.Stderr, "warning: no devbox config; removed warp live record\n")
			return nil
		}
		cfg, err := config.LoadDefault()
		if err != nil {
			// The config exists but cannot be loaded (typo, bad YAML, a reserved
			// name). Keep the record: the user will fix the config, and the next
			// shell in this tab restores. Only a missing config or a deleted profile
			// makes the record meaningless.
			fmt.Fprintf(os.Stderr, "warning: warp restore: %v\n", err)
			return nil
		}
		prof, ok := cfg.Profile(profName)
		if !ok {
			warp.ClearLive(dir, id)
			fmt.Fprintf(os.Stderr, "warning: profile %q no longer exists; removed warp live record\n", profName)
			return nil
		}
		host, client = prof.Host, newClient(prof)
	}

	fmt.Printf("Restoring %s:%s\n", profName, session)
	recordProfileSession(profName, session)
	err = execTmuxSession(client, profName, host, session)
	// Prune only now, after this tab's attach has ended. Warp restores every
	// tab at once; pruning before the load would let one tab delete a
	// sibling's week-old record before that sibling reads it. By the time
	// this returns, every sibling that restored has re-saved a fresh record.
	warp.PruneLive(dir, liveRecordMaxAge)
	return err
}
