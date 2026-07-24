package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndGetSession(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")

	if err := SaveSession(sessDir, "builder-104", "ttys003", "work"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	name, err := GetSession(sessDir, "builder-104", "ttys003")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if name != "work" {
		t.Errorf("GetSession = %q, want %q", name, "work")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := GetSession(dir, "builder-104", "ttys999")
	if err == nil {
		t.Error("GetSession should fail for unknown TTY")
	}
}

func TestGetSession_ScopedPerProfile(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	SaveSession(sessDir, "builder-104", "ttys003", "work")
	// same TTY, different profile — must not see the other profile's session
	if _, err := GetSession(sessDir, "aaa", "ttys003"); err == nil {
		t.Error("GetSession should not find a record from another profile")
	}
}

func TestSaveSession_OwnerOnlyPerms(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := SaveSession(sessDir, "builder-104", "ttys003", "work"); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	info, err := os.Stat(filepath.Join(sessDir, "builder-104", "ttys003"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file perms = %o, want 600", perm)
	}
}

func TestDeleteSessionsByName(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	SaveSession(sessDir, "builder-104", "ttys001", "api")
	SaveSession(sessDir, "builder-104", "ttys002", "work")
	SaveSession(sessDir, "builder-104", "ttys003", "api")

	DeleteSessionsByName(sessDir, "builder-104", "api")

	if _, err := GetSession(sessDir, "builder-104", "ttys001"); err == nil {
		t.Error("ttys001 record should be removed (pointed at killed session)")
	}
	if _, err := GetSession(sessDir, "builder-104", "ttys003"); err == nil {
		t.Error("ttys003 record should be removed (pointed at killed session)")
	}
	if name, err := GetSession(sessDir, "builder-104", "ttys002"); err != nil || name != "work" {
		t.Errorf("ttys002 record should survive, got %q err %v", name, err)
	}
}

func TestDeleteSessionsByName_OnlyTargetProfile(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	SaveSession(sessDir, "builder-104", "ttys001", "api")
	SaveSession(sessDir, "aaa", "ttys002", "api") // same session name, other profile

	DeleteSessionsByName(sessDir, "builder-104", "api")

	if _, err := GetSession(sessDir, "aaa", "ttys002"); err != nil {
		t.Error("a same-named session under another profile must not be deleted")
	}
}

func TestDeleteSessionsByName_MissingDir(t *testing.T) {
	// Must not panic when the sessions dir doesn't exist yet
	DeleteSessionsByName(filepath.Join(t.TempDir(), "nope"), "builder-104", "api")
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
