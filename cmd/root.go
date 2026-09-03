package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hiennguyenle/devbox/internal/config"
	"github.com/hiennguyenle/devbox/internal/local"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
	"github.com/hiennguyenle/devbox/internal/tmux"
	"github.com/hiennguyenle/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var (
	verbose     bool
	profileFlag string
	localFlag   bool
	version     = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "devbox [PROFILE]",
	Short:   "Manage tmux sessions over SSH or locally",
	Version: version,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runRoot,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print the commands being executed (ssh or local)")
	rootCmd.PersistentFlags().StringVarP(&profileFlag, "profile", "p", "", "profile to act on (overrides last-used/default)")
	rootCmd.PersistentFlags().BoolVar(&localFlag, "local", false, "act on tmux sessions on this machine instead of a remote profile")
}

func Execute() error {
	return rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) error {
	explicit := ""
	if len(args) == 1 {
		explicit = args[0]
	}
	name, host, defSession, client, err := resolveExecutor(explicit)
	if err != nil {
		return err
	}
	recordProfileSession(name, defSession)
	return execTmuxSession(client, host, defSession, name == localProfileName)
}

func requireConfig() (*config.Config, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("no profiles configured. Run \"devbox profile\" first")
	}
	return config.LoadDefault()
}

// newClient is a var so tests can inject a MockExecutor.
var newClient = func(prof *config.Profile) sshpkg.Executor {
	return &sshpkg.Client{
		Host:    prof.Host,
		User:    prof.User,
		Port:    prof.Port,
		Verbose: verbose,
	}
}

// localProfileName is the built-in profile for tmux on this machine. It is
// reserved (see internal/validate) and never appears in config.yaml.
const localProfileName = "local"

// localDefaultSession is what bare devbox --local attaches to.
const localDefaultSession = "work"

// newLocalClient is a var so tests can inject a MockExecutor.
var newLocalClient = func() sshpkg.Executor {
	return &local.Executor{Verbose: verbose}
}

// resolveExecutor decides this invocation's transport and returns the acting
// profile name, a host label for messages, the session to attach when none
// is named, and the executor. --local (or positional "local" at the root)
// needs no config; otherwise config is required and resolved normally.
func resolveExecutor(explicit string) (name, host, defSession string, client sshpkg.Executor, err error) {
	if localFlag || explicit == localProfileName {
		if profileFlag != "" || (explicit != "" && explicit != localProfileName) {
			return "", "", "", nil, fmt.Errorf("--local cannot be combined with a profile")
		}
		return localProfileName, "localhost", localDefaultSession, newLocalClient(), nil
	}
	cfg, err := requireConfig()
	if err != nil {
		return "", "", "", nil, err
	}
	name, prof, err := resolveProfile(cfg, explicit)
	if err != nil {
		return "", "", "", nil, err
	}
	return name, prof.Host, prof.DefaultSession, newClient(prof), nil
}

// handleSSHError classifies a failed non-interactive Run from its captured
// output ("command not found" only ever appears there, not in err.Error()).
func handleSSHError(output string, err error, host string, local bool) error {
	if err == nil {
		return nil
	}
	if strings.Contains(output, "command not found") {
		if local {
			return fmt.Errorf("tmux is not installed locally")
		}
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	if local {
		return fmt.Errorf("local command failed: %w", err)
	}
	return fmt.Errorf("cannot reach %s. Check your connection or run \"devbox profile\"", host)
}

// handleExecErr classifies a failed interactive Exec. Output went straight to
// the terminal, so only the exit code is available: 127 means the remote
// shell couldn't find tmux.
func handleExecErr(err error, host string, local bool) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 127 {
		if local {
			return fmt.Errorf("tmux is not installed locally")
		}
		return fmt.Errorf("tmux is not installed on %s", host)
	}
	return err
}

func execTmuxSession(client sshpkg.Executor, host, sessionName string, local bool) error {
	hintShown := false
	showResumeHint := func() {
		if hintShown {
			return
		}
		hintShown = true
		resumeCmd := "devbox resume"
		if local {
			resumeCmd = "devbox resume --local"
		}
		if validate.SessionName(sessionName) != nil {
			fmt.Printf("To continue this session, run %s\n", resumeCmd)
			return
		}
		fmt.Printf("To continue this session, run %s %s\n", resumeCmd, sessionName)
	}

	if err := handleExecErr(client.Exec(tmux.NewAttachSession(sessionName), showResumeHint), host, local); err != nil {
		return err
	}
	showResumeHint()
	return nil
}
