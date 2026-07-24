package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndGetLastProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lastprofile")
	if err := SaveLastProfile(dir, "ttys001", "builder-104"); err != nil {
		t.Fatalf("SaveLastProfile: %v", err)
	}
	got, err := GetLastProfile(dir, "ttys001")
	if err != nil {
		t.Fatalf("GetLastProfile: %v", err)
	}
	if got != "builder-104" {
		t.Errorf("GetLastProfile = %q, want %q", got, "builder-104")
	}
}

func TestGetLastProfile_Missing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lastprofile")
	if _, err := GetLastProfile(dir, "ttys999"); err == nil {
		t.Error("GetLastProfile should error when no record exists")
	}
}

func TestGetLastProfile_Empty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lastprofile")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "ttys002"), []byte("   \n"), 0600)
	if _, err := GetLastProfile(dir, "ttys002"); err == nil {
		t.Error("GetLastProfile should error on empty record")
	}
}

func TestSaveLastProfile_OverwritesPerTTY(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lastprofile")
	SaveLastProfile(dir, "ttys003", "one")
	SaveLastProfile(dir, "ttys003", "two")
	got, err := GetLastProfile(dir, "ttys003")
	if err != nil {
		t.Fatalf("GetLastProfile: %v", err)
	}
	if got != "two" {
		t.Errorf("GetLastProfile = %q, want %q (latest write wins)", got, "two")
	}
}
