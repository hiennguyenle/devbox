package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DefaultProfile: "builder-104",
		Profiles: map[string]*Profile{
			"builder-104": {Host: "builder-104", User: "hien", Port: 22, DefaultSession: "work"},
		},
	}

	path := filepath.Join(dir, "config.yaml")
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	p, ok := loaded.Profile("builder-104")
	if !ok {
		t.Fatalf("profile builder-104 missing after load")
	}
	if p.Host != "builder-104" {
		t.Errorf("Host = %q, want %q", p.Host, "builder-104")
	}
	if p.User != "hien" {
		t.Errorf("User = %q, want %q", p.User, "hien")
	}
	if p.Port != 22 {
		t.Errorf("Port = %d, want %d", p.Port, 22)
	}
	if p.DefaultSession != "work" {
		t.Errorf("DefaultSession = %q, want %q", p.DefaultSession, "work")
	}
	if loaded.DefaultProfile != "builder-104" {
		t.Errorf("DefaultProfile = %q, want %q", loaded.DefaultProfile, "builder-104")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load should fail for missing file")
	}
}

func TestLoad_ProfileDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("profiles:\n  box:\n    host: myhost\n"), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.Profile("box")
	if !ok {
		t.Fatalf("profile box missing")
	}
	if p.Port != 22 {
		t.Errorf("Port default = %d, want 22", p.Port)
	}
	if p.DefaultSession != "work" {
		t.Errorf("DefaultSession default = %q, want %q", p.DefaultSession, "work")
	}
}

func TestLoad_MigratesLegacySingleHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("host: dev.example.com\nuser: hien\nport: 2222\ndefault_session: main\n"), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProfile != "dev" {
		t.Fatalf("migrated DefaultProfile = %q, want %q", cfg.DefaultProfile, "dev")
	}
	p, ok := cfg.Profile("dev")
	if !ok {
		t.Fatalf("migrated profile dev missing")
	}
	if p.Host != "dev.example.com" || p.User != "hien" || p.Port != 2222 || p.DefaultSession != "main" {
		t.Errorf("migrated profile = %+v, want host/user/port/session preserved", p)
	}
}

func TestLoad_MigrationIPHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("host: 10.0.0.5\n"), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// first label of 10.0.0.5 is "10" — valid, alphanumeric-leading
	if cfg.DefaultProfile != "10" {
		t.Errorf("migrated DefaultProfile = %q, want %q", cfg.DefaultProfile, "10")
	}
}

func TestSave_DropsLegacyFieldsAfterMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("host: dev.example.com\nuser: hien\n"), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out := filepath.Join(dir, "out.yaml")
	if err := Save(cfg, out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, _ := os.ReadFile(out)
	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fc.Host != "" {
		t.Errorf("saved config still has top-level host %q", fc.Host)
	}
	if len(fc.Profiles) == 0 {
		t.Errorf("saved config has no profiles")
	}
}

func TestSave_OwnerOnlyPerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &Config{DefaultProfile: "h", Profiles: map[string]*Profile{"h": {Host: "h"}}}
	if err := Save(cfg, path); err != nil {
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
	for _, yaml := range []string{
		"profiles:\n  box:\n    host: h\n    port: 70000\n",
		"profiles:\n  box:\n    host: h\n    port: -5\n",
	} {
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
		"profiles:\n  box:\n    host: \"-oProxyCommand=evil\"\n",
		"user: hien\n",   // legacy shape but no host -> no profiles
		"profiles: {}\n", // empty profiles map
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

func TestLoad_DefaultProfileBackfill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// two profiles, no default_profile -> first alphabetically becomes default
	os.WriteFile(path, []byte("profiles:\n  zeta:\n    host: z\n  alpha:\n    host: a\n"), 0600)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProfile != "alpha" {
		t.Errorf("DefaultProfile back-fill = %q, want %q", cfg.DefaultProfile, "alpha")
	}
}

func TestLoad_MissingDefaultProfileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("default_profile: ghost\nprofiles:\n  real:\n    host: r\n"), 0600)

	if _, err := Load(path); err == nil {
		t.Error("Load should reject a default_profile that names no profile")
	}
}

func TestProfileNames_Sorted(t *testing.T) {
	cfg := &Config{Profiles: map[string]*Profile{
		"zeta": {Host: "z"}, "alpha": {Host: "a"}, "mid": {Host: "m"},
	}}
	got := cfg.ProfileNames()
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("ProfileNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ProfileNames()[%d] = %q, want %q", i, got[i], want[i])
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
