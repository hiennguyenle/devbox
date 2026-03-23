package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndGetSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")

	if err := SaveSession(sessDir, "ttys003", "work"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	name, err := GetSession(sessDir, "ttys003")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if name != "work" {
		t.Errorf("GetSession = %q, want %q", name, "work")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := GetSession(dir, "ttys999")
	if err == nil {
		t.Error("GetSession should fail for unknown TTY")
	}
}

func TestDeleteSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	SaveSession(sessDir, "ttys003", "work")
	DeleteSession(sessDir, "ttys003")

	_, err := GetSession(sessDir, "ttys003")
	if err == nil {
		t.Error("GetSession should fail after delete")
	}
}

func TestLastSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")

	SaveSession(sessDir, "ttys001", "api")
	time.Sleep(10 * time.Millisecond)
	SaveSession(sessDir, "ttys002", "work")

	name, err := LastSession(sessDir)
	if err != nil {
		t.Fatalf("LastSession: %v", err)
	}
	if name != "work" {
		t.Errorf("LastSession = %q, want %q", name, "work")
	}
}

func TestLastSession_Empty(t *testing.T) {
	dir := t.TempDir()
	_, err := LastSession(dir)
	if err == nil {
		t.Error("LastSession should fail with no sessions")
	}
}

func TestResolveTTY(t *testing.T) {
	name, err := ResolveTTY()
	if err != nil {
		t.Skipf("no TTY available: %v", err)
	}
	if name == "" {
		t.Error("ResolveTTY returned empty string")
	}
}
