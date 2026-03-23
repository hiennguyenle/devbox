package validate

import (
	"fmt"
	"regexp"
)

var sessionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func SessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf("session name too long (max 50 characters)")
	}
	if !sessionNamePattern.MatchString(name) {
		return fmt.Errorf("session name %q contains invalid characters (use alphanumeric, hyphens, underscores)", name)
	}
	return nil
}
