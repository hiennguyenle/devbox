package warp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiennguyenle/devbox/internal/config"
)

func TestLive_SaveLoadClear(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "live") // SaveLive must create it
	id := "aca3a00ac45440f19afea15ac826acd0"

	if err := SaveLive(dir, id, "builder-104", "api"); err != nil {
		t.Fatalf("SaveLive: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, id)); string(data) != "builder-104\napi\n" {
		t.Errorf("content = %q, want profile and session lines", data)
	}

	p, s, err := LoadLive(dir, id)
	if err != nil || p != "builder-104" || s != "api" {
		t.Errorf("LoadLive = %q, %q, %v; want builder-104, api, nil", p, s, err)
	}

	ClearLive(dir, id)
	ClearLive(dir, id) // second clear is a no-op, never an error
	if _, _, err := LoadLive(dir, id); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after clear: err = %v, want not-exist", err)
	}
}

func TestLoadLive_MalformedIsNotNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc"), []byte("only-one-line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadLive(dir, "abc")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want a malformed-record error distinct from not-exist", err)
	}
}

func TestPruneLive_RemovesOnlyOldRecords(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"old", "new"} {
		if err := SaveLive(dir, id, "p", "s"); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old"), past, past); err != nil {
		t.Fatal(err)
	}

	PruneLive(dir, 7*24*time.Hour)

	if _, err := os.Stat(filepath.Join(dir, "old")); !os.IsNotExist(err) {
		t.Error("old record should be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); err != nil {
		t.Error("fresh record should survive")
	}
}

func TestPruneLive_MissingDirIsNoop(t *testing.T) {
	PruneLive(filepath.Join(t.TempDir(), "nope"), time.Hour) // must not panic
}

func TestLiveDir_UnderConfigDir(t *testing.T) {
	config.SetDirOverride("/cfg")
	t.Cleanup(func() { config.SetDirOverride("") })
	if got := LiveDir(); got != filepath.Join("/cfg", "warp", "live") {
		t.Errorf("LiveDir() = %q", got)
	}
}
