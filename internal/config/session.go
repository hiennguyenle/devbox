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

func SaveSession(sessDir, ttyName, sessionName string) error {
	if err := os.MkdirAll(sessDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(sessDir, ttyName)
	return os.WriteFile(path, []byte(sessionName), 0600)
}

func GetSession(sessDir, ttyName string) (string, error) {
	path := filepath.Join(sessDir, ttyName)
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

// DeleteSessionsByName removes every per-terminal record that points at
// sessionName, so a killed session stops being pre-selected on resume.
func DeleteSessionsByName(sessDir, sessionName string) {
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(sessDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == sessionName {
			os.Remove(path)
		}
	}
}

func ResolveTTY() (string, error) {
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
