package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func SessionsDir() string {
	return filepath.Join(Dir(), "sessions")
}

func SaveSession(sessDir, ttyName, sessionName string) error {
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(sessDir, ttyName)
	return os.WriteFile(path, []byte(sessionName), 0644)
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

func DeleteSession(sessDir, ttyName string) {
	path := filepath.Join(sessDir, ttyName)
	os.Remove(path)
}

func LastSession(sessDir string) (string, error) {
	entries, err := os.ReadDir(sessDir)
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("no session history")
	}

	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UnixNano() > newestTime {
			newestTime = info.ModTime().UnixNano()
			path := filepath.Join(sessDir, e.Name())
			data, err := os.ReadFile(path)
			if err == nil {
				newest = strings.TrimSpace(string(data))
			}
		}
	}

	if newest == "" {
		return "", fmt.Errorf("no session history")
	}
	return newest, nil
}

func ResolveTTY() (string, error) {
	out, err := exec.Command("tty").Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine terminal: %w", err)
	}
	ttyPath := strings.TrimSpace(string(out))
	// /dev/ttys003 -> ttys003
	return filepath.Base(ttyPath), nil
}
