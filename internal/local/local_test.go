package local

import (
	"errors"
	"os/exec"
	"testing"
)

func TestRunCapturesOutput(t *testing.T) {
	out, err := (&Executor{}).Run("echo hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hi" {
		t.Errorf("out = %q, want %q", out, "hi")
	}
}

func TestRunExitError(t *testing.T) {
	_, err := (&Executor{}).Run("exit 3")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("want exit code 3, got %v", err)
	}
}

func TestExecExitCodes(t *testing.T) {
	if err := (&Executor{}).Exec("true", nil); err != nil {
		t.Errorf("Exec true: %v", err)
	}
	if err := (&Executor{}).Exec("false", nil); err == nil {
		t.Error("Exec false: want error, got nil")
	}
}

func TestExecEmptyUsesShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/true")
	if err := (&Executor{}).Exec("", nil); err != nil {
		t.Errorf("Exec empty: %v", err)
	}
}
