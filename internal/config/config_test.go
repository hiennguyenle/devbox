package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Host:           "builder-104",
		User:           "hien",
		Port:           22,
		DefaultSession: "work",
	}

	path := filepath.Join(dir, "config.yaml")
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Host != "builder-104" {
		t.Errorf("Host = %q, want %q", loaded.Host, "builder-104")
	}
	if loaded.User != "hien" {
		t.Errorf("User = %q, want %q", loaded.User, "hien")
	}
	if loaded.Port != 22 {
		t.Errorf("Port = %d, want %d", loaded.Port, 22)
	}
	if loaded.DefaultSession != "work" {
		t.Errorf("DefaultSession = %q, want %q", loaded.DefaultSession, "work")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load should fail for missing file")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("host: myhost\n"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 22 {
		t.Errorf("Port default = %d, want 22", cfg.Port)
	}
	if cfg.DefaultSession != "work" {
		t.Errorf("DefaultSession default = %q, want %q", cfg.DefaultSession, "work")
	}
}

func TestSave_OwnerOnlyPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := Save(&Config{Host: "h"}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perms = %o, want 600", perm)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	for _, yaml := range []string{"host: h\nport: 70000\n", "host: h\nport: -5\n"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		os.WriteFile(path, []byte(yaml), 0600)
		if _, err := Load(path); err == nil {
			t.Errorf("Load should reject %q", yaml)
		}
	}
}

func TestLoad_InvalidHost(t *testing.T) {
	invalid := []string{
		"host: \"-oProxyCommand=evil\"\n",
		"user: hien\n", // missing host entirely
	}
	for _, yaml := range invalid {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		os.WriteFile(path, []byte(yaml), 0600)
		if _, err := Load(path); err == nil {
			t.Errorf("Load should reject %q", yaml)
		}
	}
}

func TestConfigDir(t *testing.T) {
	dir := t.TempDir()
	original := configDirOverride
	configDirOverride = dir
	defer func() { configDirOverride = original }()

	got := Dir()
	if got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}
