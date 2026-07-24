package cmd

import (
	"testing"

	"github.com/hiennguyenle/devbox/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		DefaultProfile: "builder-104",
		Profiles: map[string]*config.Profile{
			"builder-104": {Host: "b104", Port: 22, DefaultSession: "work"},
			"aaa":         {Host: "hostA", Port: 22, DefaultSession: "main"},
		},
	}
}

func TestResolveProfile_ExplicitPositional(t *testing.T) {
	useTempConfigDir(t)
	cfg := testConfig()
	name, prof, err := resolveProfile(cfg, "aaa")
	if err != nil {
		t.Fatalf("resolveProfile: %v", err)
	}
	if name != "aaa" || prof.Host != "hostA" {
		t.Errorf("got (%q, %q), want (aaa, hostA)", name, prof.Host)
	}
}

func TestResolveProfile_FlagWinsOverPositional(t *testing.T) {
	useTempConfigDir(t)
	profileFlag = "aaa"
	t.Cleanup(func() { profileFlag = "" })

	name, _, err := resolveProfile(testConfig(), "builder-104")
	if err != nil {
		t.Fatalf("resolveProfile: %v", err)
	}
	if name != "aaa" {
		t.Errorf("flag should win, got %q, want aaa", name)
	}
}

func TestResolveProfile_FallsBackToDefault(t *testing.T) {
	useTempConfigDir(t) // fresh dir -> no last-profile record
	name, _, err := resolveProfile(testConfig(), "")
	if err != nil {
		t.Fatalf("resolveProfile: %v", err)
	}
	if name != "builder-104" {
		t.Errorf("got %q, want default builder-104", name)
	}
}

func TestResolveProfile_UsesLastUsed(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttytest")
	if err := config.SaveLastProfile(config.LastProfileDir(), "ttytest", "aaa"); err != nil {
		t.Fatalf("SaveLastProfile: %v", err)
	}
	name, _, err := resolveProfile(testConfig(), "")
	if err != nil {
		t.Fatalf("resolveProfile: %v", err)
	}
	if name != "aaa" {
		t.Errorf("got %q, want last-used aaa", name)
	}
}

func TestResolveProfile_StaleLastUsedFallsBackToDefault(t *testing.T) {
	useTempConfigDir(t)
	fakeTTY(t, "ttytest")
	// Record a profile that no longer exists in the config.
	config.SaveLastProfile(config.LastProfileDir(), "ttytest", "ghost")
	name, _, err := resolveProfile(testConfig(), "")
	if err != nil {
		t.Fatalf("resolveProfile: %v", err)
	}
	if name != "builder-104" {
		t.Errorf("stale last-used should fall back to default, got %q", name)
	}
}

func TestResolveProfile_UnknownErrors(t *testing.T) {
	useTempConfigDir(t)
	if _, _, err := resolveProfile(testConfig(), "nope"); err == nil {
		t.Error("resolveProfile should reject an unknown profile name")
	}
}
