package validate

import (
	"fmt"
	"regexp"
)

var sessionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Both patterns require a leading alphanumeric so values can never be
// parsed as an ssh option (leading "-") and stay a single argv token.
// Hosts additionally allow ":" for IPv6 literals and "%" for zone suffixes
// (fe80::1%en0).
var (
	hostPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:%-]*$`)
	userPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

func Host(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("invalid host %q (use alphanumeric, dots, hyphens, underscores, colons; must not start with a dash)", host)
	}
	return nil
}

func User(user string) error {
	if user == "" {
		return fmt.Errorf("user cannot be empty")
	}
	if !userPattern.MatchString(user) {
		return fmt.Errorf("invalid user %q (use alphanumeric, dots, hyphens, underscores; must not start with a dash)", user)
	}
	return nil
}

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
