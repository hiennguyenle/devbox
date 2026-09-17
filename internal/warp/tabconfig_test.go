package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiennguyenle/devbox/internal/config"
)

func TestRenderProfile(t *testing.T) {
	got := RenderProfile("builder-104", &config.Profile{Host: "builder-104", DefaultSession: "work"}, "green")
	want := Marker + `
name = "devbox: builder-104"
color = "green"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} -p builder-104"]

[params.session]
description = "tmux session on builder-104"
default = "work"
`
	if got != want {
		t.Errorf("RenderProfile =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderLocal(t *testing.T) {
	got := RenderLocal()
	want := Marker + `
name = "devbox: local"
color = "blue"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} --local"]

[params.session]
description = "tmux session on this machine"
default = "work"
`
	if got != want {
		t.Errorf("RenderLocal =\n%s\nwant\n%s", got, want)
	}
}

func TestColorFor_CyclesFiveColors(t *testing.T) {
	want := []string{"green", "yellow", "magenta", "cyan", "red", "green"}
	for i, w := range want {
		if got := ColorFor(i); got != w {
			t.Errorf("ColorFor(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestSync_WritesProfilesThenLocal(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DefaultProfile: "b", Profiles: map[string]*config.Profile{
		"b": {Host: "hb", DefaultSession: "work"},
		"a": {Host: "ha", DefaultSession: "main"},
	}}

	changes, err := Sync(dir, cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	wantPaths := []string{
		filepath.Join(dir, "devbox_a.toml"),
		filepath.Join(dir, "devbox_b.toml"),
		filepath.Join(dir, "devbox_local.toml"),
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %+v, want 3", changes)
	}
	for i, c := range changes {
		if c.Action != ActionWrote || c.Path != wantPaths[i] {
			t.Errorf("changes[%d] = %+v, want wrote %s", i, c, wantPaths[i])
		}
	}
	// sorted profile order decides the color: a=green, b=yellow
	if data, _ := os.ReadFile(wantPaths[0]); string(data) != RenderProfile("a", cfg.Profiles["a"], "green") {
		t.Errorf("devbox_a.toml content mismatch:\n%s", data)
	}
	if data, _ := os.ReadFile(wantPaths[1]); string(data) != RenderProfile("b", cfg.Profiles["b"], "yellow") {
		t.Errorf("devbox_b.toml content mismatch:\n%s", data)
	}
	if data, _ := os.ReadFile(wantPaths[2]); string(data) != RenderLocal() {
		t.Errorf("devbox_local.toml content mismatch:\n%s", data)
	}
	if info, _ := os.Stat(wantPaths[0]); info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want 0644", info.Mode().Perm())
	}
}

func TestSync_NilConfigWritesLocalAndRemovesStaleManaged(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "devbox_old.toml")
	if err := os.WriteFile(stale, []byte(Marker+"\nname = \"x\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	changes, err := Sync(dir, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	want := []Change{
		{ActionWrote, filepath.Join(dir, "devbox_local.toml")},
		{ActionRemoved, stale},
	}
	if len(changes) != 2 || changes[0] != want[0] || changes[1] != want[1] {
		t.Errorf("changes = %+v, want %+v", changes, want)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale managed file should be removed")
	}
}

func TestSync_SkipsUnmanagedCollision(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "devbox_a.toml")
	hand := "name = \"hand written\"\n"
	if err := os.WriteFile(mine, []byte(hand), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Profiles: map[string]*config.Profile{"a": {Host: "h", DefaultSession: "work"}}}

	changes, err := Sync(dir, cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(changes) < 1 || changes[0] != (Change{ActionSkipped, mine}) {
		t.Errorf("changes[0] = %+v, want skipped %s", changes, mine)
	}
	if data, _ := os.ReadFile(mine); string(data) != hand {
		t.Error("unmanaged file must not be overwritten")
	}
}

func TestSync_LeavesUnmanagedStaleFilesAlone(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "devbox_other.toml")
	if err := os.WriteFile(other, []byte("name = \"hand written\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("unmanaged file with devbox_ prefix must survive sync")
	}
}

func TestSync_RewritesChangedManagedFile(t *testing.T) {
	dir := t.TempDir()
	prof := &config.Profile{Host: "ha", DefaultSession: "work"}
	cfg := &config.Config{DefaultProfile: "a", Profiles: map[string]*config.Profile{"a": prof}}
	if _, err := Sync(dir, cfg); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	prof.Host = "hb"
	prof.DefaultSession = "api"
	changes, err := Sync(dir, cfg)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	path := filepath.Join(dir, "devbox_a.toml")
	found := false
	for _, c := range changes {
		if c.Path == path {
			found = true
			if c.Action != ActionWrote {
				t.Errorf("changes for %s = %q, want %q", path, c.Action, ActionWrote)
			}
		}
	}
	if !found {
		t.Errorf("changes = %+v, want one for %s", changes, path)
	}
	want := RenderProfile("a", prof, "green")
	if data, _ := os.ReadFile(path); string(data) != want {
		t.Errorf("devbox_a.toml after the change:\n%s\nwant\n%s", data, want)
	}
}

func TestSync_MissingDirErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := Sync(missing, nil)
	if err == nil || !strings.Contains(err.Error(), "warp tab configs directory not found: "+missing) {
		t.Errorf("err = %v, want directory-not-found naming %s", err, missing)
	}
}
