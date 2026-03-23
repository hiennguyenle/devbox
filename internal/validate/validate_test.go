package validate

import "testing"

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
		{string(make([]byte, 51)), "too long"},
	}
	for _, tc := range invalid {
		if err := SessionName(tc.name); err == nil {
			t.Errorf("SessionName(%q) should fail (%s), got nil", tc.name, tc.reason)
		}
	}
}
