package validate

import (
	"strings"
	"testing"
)

func TestProfileName_Valid(t *testing.T) {
	valid := []string{"builder-104", "aaa", "work", "a", "host_1", "s-1a2b", "1"}
	for _, name := range valid {
		if err := ProfileName(name); err != nil {
			t.Errorf("ProfileName(%q) should be valid, got: %v", name, err)
		}
	}
}

func TestProfileName_Invalid(t *testing.T) {
	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"-foo", "leading dash"},
		{"a b", "space"},
		{"a.b", "dot"},
		{"a/b", "slash"},
		{"user@host", "at sign"},
		{strings.Repeat("a", 51), "too long"},
	}
	for _, tc := range invalid {
		if err := ProfileName(tc.name); err == nil {
			t.Errorf("ProfileName(%q) should fail (%s), got nil", tc.name, tc.reason)
		}
	}
}

func TestProfileName_ReservedRejected(t *testing.T) {
	reserved := []string{"list", "new", "resume", "kill", "rename", "shell", "version", "profile", "connect", "help", "completion", "local", "warp"}
	for _, name := range reserved {
		if err := ProfileName(name); err == nil {
			t.Errorf("ProfileName(%q) should be rejected as reserved, got nil", name)
		}
	}
}

func TestSessionName_Valid(t *testing.T) {
	valid := []string{"work", "api", "my-session", "test_123", "a"}
	for _, name := range valid {
		if err := SessionName(name); err != nil {
			t.Errorf("SessionName(%q) should be valid, got: %v", name, err)
		}
	}
}

func TestSessionName_Invalid(t *testing.T) {
	invalid := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"hello world", "spaces"},
		{"my/session", "slash"},
		{"name@host", "at sign"},
		{"a.b", "dot"},
		{strings.Repeat("a", 51), "too long"},
	}
	for _, tc := range invalid {
		if err := SessionName(tc.name); err == nil {
			t.Errorf("SessionName(%q) should fail (%s), got nil", tc.name, tc.reason)
		}
	}
}

func TestSessionName_TooLong_HitsLengthCheck(t *testing.T) {
	// 51 valid chars: must fail on length, not the character class
	err := SessionName(strings.Repeat("a", 51))
	if err == nil {
		t.Fatal("expected error for 51-char name")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected length error, got: %v", err)
	}
}

func TestHost_Valid(t *testing.T) {
	valid := []string{"example.com", "192.168.1.10", "my-server", "dev.internal.corp", "host_1", "2001:db8::1", "fe80::1", "fe80::1%en0"}
	for _, h := range valid {
		if err := Host(h); err != nil {
			t.Errorf("Host(%q) should be valid, got: %v", h, err)
		}
	}
}

func TestHost_Invalid(t *testing.T) {
	invalid := []struct {
		host   string
		reason string
	}{
		{"", "empty"},
		{"-oProxyCommand=evil", "leading dash parses as ssh option"},
		{"host name", "whitespace"},
		{"host\tname", "tab"},
		{"user@host", "at sign belongs to user part"},
		{"host;rm", "shell metacharacter"},
	}
	for _, tc := range invalid {
		if err := Host(tc.host); err == nil {
			t.Errorf("Host(%q) should fail (%s), got nil", tc.host, tc.reason)
		}
	}
}

func TestUser_Valid(t *testing.T) {
	valid := []string{"root", "hien", "deploy-bot", "user_1", "a.b"}
	for _, u := range valid {
		if err := User(u); err != nil {
			t.Errorf("User(%q) should be valid, got: %v", u, err)
		}
	}
}

func TestUser_Invalid(t *testing.T) {
	invalid := []struct {
		user   string
		reason string
	}{
		{"", "empty"},
		{"-flag", "leading dash"},
		{"user name", "whitespace"},
		{"user@host", "at sign"},
	}
	for _, tc := range invalid {
		if err := User(tc.user); err == nil {
			t.Errorf("User(%q) should fail (%s), got nil", tc.user, tc.reason)
		}
	}
}
