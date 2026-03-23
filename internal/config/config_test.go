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
