package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hien/devbox/internal/validate"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Host           string `yaml:"host"`
	User           string `yaml:"user,omitempty"`
	Port           int    `yaml:"port,omitempty"`
	DefaultSession string `yaml:"default_session,omitempty"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no config found: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.DefaultSession == "" {
		cfg.DefaultSession = "work"
	}
	if err := validate.Host(cfg.Host); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.User != "" {
		if err := validate.User(cfg.User); err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("config: invalid port %d (must be 1-65535)", cfg.Port)
	}
	if err := validate.SessionName(cfg.DefaultSession); err != nil {
		return nil, fmt.Errorf("config: default_session: %w", err)
	}
	return cfg, nil
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
