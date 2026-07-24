package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hiennguyenle/devbox/internal/validate"
	"gopkg.in/yaml.v3"
)

// Profile is one remote host devbox can connect to.
type Profile struct {
	Host           string `yaml:"host"`
	User           string `yaml:"user,omitempty"`
	Port           int    `yaml:"port,omitempty"`
	DefaultSession string `yaml:"default_session,omitempty"`
}

// Config is the set of named profiles plus which one is the default.
type Config struct {
	DefaultProfile string              `yaml:"default_profile,omitempty"`
	Profiles       map[string]*Profile `yaml:"profiles,omitempty"`
}

// fileConfig captures both the current multi-profile shape and the legacy
// single-host shape so Load can migrate old files transparently.
type fileConfig struct {
	DefaultProfile string              `yaml:"default_profile"`
	Profiles       map[string]*Profile `yaml:"profiles"`

	// legacy single-host fields, migrated on load and never written back
	Host           string `yaml:"host"`
	User           string `yaml:"user"`
	Port           int    `yaml:"port"`
	DefaultSession string `yaml:"default_session"`
}

var configDirOverride string

func Dir() string {
	if configDirOverride != "" {
		return configDirOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "devbox")
}

func SetDirOverride(dir string) {
	configDirOverride = dir
}

func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

// Profile returns the named profile, if present.
func (c *Config) Profile(name string) (*Profile, bool) {
	p, ok := c.Profiles[name]
	return p, ok
}

// ProfileNames returns the profile names sorted alphabetically, for stable
// listing and deterministic default back-fill.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no config found: %w", err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg := &Config{DefaultProfile: fc.DefaultProfile, Profiles: fc.Profiles}

	// Migrate a legacy single-host config into a one-profile config.
	if len(cfg.Profiles) == 0 && fc.Host != "" {
		name := profileNameFromHost(fc.Host)
		cfg.Profiles = map[string]*Profile{
			name: {Host: fc.Host, User: fc.User, Port: fc.Port, DefaultSession: fc.DefaultSession},
		}
		cfg.DefaultProfile = name
	}

	if len(cfg.Profiles) == 0 {
		return nil, fmt.Errorf("config has no profiles")
	}

	for name, p := range cfg.Profiles {
		if p == nil {
			return nil, fmt.Errorf("config: profile %q is empty", name)
		}
		if p.Port == 0 {
			p.Port = 22
		}
		if p.DefaultSession == "" {
			p.DefaultSession = "work"
		}
		if err := validate.ProfileName(name); err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		if err := validate.Host(p.Host); err != nil {
			return nil, fmt.Errorf("config: profile %q: %w", name, err)
		}
		if p.User != "" {
			if err := validate.User(p.User); err != nil {
				return nil, fmt.Errorf("config: profile %q: %w", name, err)
			}
		}
		if p.Port < 1 || p.Port > 65535 {
			return nil, fmt.Errorf("config: profile %q: invalid port %d (must be 1-65535)", name, p.Port)
		}
		if err := validate.SessionName(p.DefaultSession); err != nil {
			return nil, fmt.Errorf("config: profile %q: default_session: %w", name, err)
		}
	}

	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = cfg.ProfileNames()[0]
	} else if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
		return nil, fmt.Errorf("config: default_profile %q is not a defined profile", cfg.DefaultProfile)
	}

	return cfg, nil
}

// profileNameFromHost derives a valid profile name from a host by taking its
// first label (before the first "." or ":") and sanitizing it. Falls back to
// "default" when nothing usable remains.
func profileNameFromHost(host string) string {
	label := host
	if i := strings.IndexAny(label, ".:"); i >= 0 {
		label = label[:i]
	}
	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.TrimLeft(b.String(), "-_")
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" || validate.ProfileName(name) != nil {
		return "default"
	}
	return name
}

func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot serialize config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func LoadDefault() (*Config, error) {
	return Load(Path())
}
