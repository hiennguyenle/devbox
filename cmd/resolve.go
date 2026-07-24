package cmd

import (
	"fmt"
	"os"

	"github.com/hiennguyenle/devbox/internal/config"
)

// resolveProfile decides which profile a command acts on. Precedence:
//
//	-p/--profile flag  >  explicit positional (root only)  >
//	last-used on this TTY  >  the configured default profile.
//
// It returns the resolved name and profile, or an error if the resolved name
// names no configured profile.
func resolveProfile(cfg *config.Config, explicit string) (string, *config.Profile, error) {
	name := explicit
	if profileFlag != "" {
		name = profileFlag
	}
	if name == "" {
		name = lastOrDefaultProfile(cfg)
	}
	prof, ok := cfg.Profile(name)
	if !ok {
		return "", nil, fmt.Errorf("unknown profile %q. Run \"devbox profile --list\"", name)
	}
	return name, prof, nil
}

// lastOrDefaultProfile returns the last profile used on this terminal, falling
// back to the configured default. A stale record (naming a profile that no
// longer exists) is ignored.
func lastOrDefaultProfile(cfg *config.Config) string {
	if tty, err := config.ResolveTTY(); err == nil {
		if name, err := config.GetLastProfile(config.LastProfileDir(), tty); err == nil {
			if _, ok := cfg.Profile(name); ok {
				return name
			}
		}
	}
	return cfg.DefaultProfile
}

// recordProfileSession remembers, for this terminal, both the profile just
// connected to and the session attached on it. Best-effort: a terminal without
// a TTY (e.g. a pipe) simply isn't tracked.
func recordProfileSession(profileName, sessionName string) {
	tty, err := config.ResolveTTY()
	if err != nil {
		return
	}
	if err := config.SaveLastProfile(config.LastProfileDir(), tty, profileName); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save last profile: %v\n", err)
	}
	if err := config.SaveSession(config.SessionsDir(), profileName, tty, sessionName); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save session state: %v\n", err)
	}
}
