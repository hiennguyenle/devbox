package warp

import (
	"strings"
	"testing"
)

func TestHook_Zsh(t *testing.T) {
	got, err := Hook("zsh")
	if err != nil {
		t.Fatal(err)
	}
	want := `# devbox: reattach this Warp tab's session after a restart
if [[ -o interactive && "$TERM_PROGRAM" == "WarpTerminal" && -n "$WARP_TERMINAL_SESSION_UUID" && "${DEVBOX_WARP_RESTORE:-1}" != "0" ]]; then
  devbox warp restore
fi
`
	if got != want {
		t.Errorf("Hook(zsh) =\n%s\nwant\n%s", got, want)
	}
}

func TestHook_BashUsesInteractiveFlagTest(t *testing.T) {
	got, err := Hook("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, guard := range []string{
		`$- == *i*`,
		`"$TERM_PROGRAM" == "WarpTerminal"`,
		`-n "$WARP_TERMINAL_SESSION_UUID"`,
		`"${DEVBOX_WARP_RESTORE:-1}" != "0"`,
		"devbox warp restore",
	} {
		if !strings.Contains(got, guard) {
			t.Errorf("Hook(bash) missing %q:\n%s", guard, got)
		}
	}
	if strings.Contains(got, "-o interactive") {
		t.Error("bash snippet must not use the zsh-only -o interactive test")
	}
}

func TestHook_UnsupportedShell(t *testing.T) {
	_, err := Hook("fish")
	if err == nil || err.Error() != `unsupported shell "fish" (zsh, bash)` {
		t.Errorf("err = %v", err)
	}
}
