package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LastProfileDir is where per-terminal last-used profile records live.
func LastProfileDir() string {
	return filepath.Join(Dir(), "lastprofile")
}

// SaveLastProfile records profileName as the last profile used on this
// terminal (keyed by ttyName).
func SaveLastProfile(dir, ttyName, profileName string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ttyName), []byte(profileName), 0600)
}

// GetLastProfile returns the last profile used on this terminal, or an error
// if no usable record exists.
func GetLastProfile(dir, ttyName string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ttyName))
	if err != nil {
		return "", fmt.Errorf("no last profile for this terminal")
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", fmt.Errorf("empty profile name")
	}
	return name, nil
}
