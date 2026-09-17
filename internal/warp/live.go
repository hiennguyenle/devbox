package warp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hiennguyenle/devbox/internal/config"
)

// LiveDir holds one record per Warp tab that currently has a devbox tmux
// attach running: <config dir>/warp/live/<WARP_TERMINAL_SESSION_UUID>.
// A record that outlives its process (reboot, Warp quit, crash) is what
// `devbox warp restore` uses to reattach the restored tab.
func LiveDir() string {
	return filepath.Join(config.Dir(), "warp", "live")
}

// SaveLive records that the Warp tab id is attached to session on profile.
func SaveLive(dir, id, profile, session string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, id), []byte(profile+"\n"+session+"\n"), 0600)
}

// LoadLive reads the record for id. A missing record returns an error that
// satisfies errors.Is(err, fs.ErrNotExist); a malformed one does not, so
// callers can tell "nothing to restore" from "remove this junk".
func LoadLive(dir, id string) (profile, session string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, id))
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", fmt.Errorf("malformed live record %s", id)
	}
	return lines[0], lines[1], nil
}

// ClearLive removes the record for id. Idempotent.
func ClearLive(dir, id string) {
	os.Remove(filepath.Join(dir, id))
}

// PruneLive removes records whose modification time is older than
// olderThan. A tab closed before a reboot never comes back, so its record
// would otherwise live forever.
func PruneLive(dir string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
