package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var safeTTYPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func SessionsDir() string {
	return filepath.Join(Dir(), "sessions")
}

// Session records are scoped per profile so a session name on one host never
// pre-selects or gets cleared alongside a same-named session on another host.
func SaveSession(sessDir, profile, ttyName, sessionName string) error {
	dir := filepath.Join(sessDir, profile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, ttyName)
	return os.WriteFile(path, []byte(sessionName), 0600)
}

func GetSession(sessDir, profile, ttyName string) (string, error) {
	path := filepath.Join(sessDir, profile, ttyName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no session for this terminal")
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", fmt.Errorf("empty session name")
	}
	return name, nil
}

// DeleteSessionsByName removes every per-terminal record under the given
// profile that points at sessionName, so a killed session stops being
// pre-selected on resume. Records for other profiles are left untouched.
func DeleteSessionsByName(sessDir, profile, sessionName string) {
	dir := filepath.Join(sessDir, profile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == sessionName {
			os.Remove(path)
		}
	}
}

// ResolveTTY is a var so tests can inject a deterministic terminal name in
// place of shelling out to `tty` (which fails without a controlling terminal).
var ResolveTTY = func() (string, error) {
	out, err := exec.Command("tty").Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine terminal: %w", err)
	}
	ttyPath := strings.TrimSpace(string(out))
	// /dev/ttys003 -> ttys003
	name := filepath.Base(ttyPath)
	if !safeTTYPattern.MatchString(name) {
		return "", fmt.Errorf("unexpected TTY name: %q", name)
	}
	return name, nil
}
