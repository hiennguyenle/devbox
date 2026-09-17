package warp

import "fmt"

// Hook returns the rc-file snippet that reattaches a restored Warp tab. It
// runs only in an interactive shell, inside Warp, with a session id, and
// with DEVBOX_WARP_RESTORE not set to 0. devbox prints it; the user pastes
// it. devbox never edits rc files.
func Hook(shell string) (string, error) {
	var interactive string
	switch shell {
	case "zsh":
		interactive = "-o interactive"
	case "bash":
		interactive = "$- == *i*"
	default:
		return "", fmt.Errorf("unsupported shell %q (zsh, bash)", shell)
	}
	return fmt.Sprintf(`# devbox: reattach this Warp tab's session after a restart
if [[ %s && "$TERM_PROGRAM" == "WarpTerminal" && -n "$WARP_TERMINAL_SESSION_UUID" && "${DEVBOX_WARP_RESTORE:-1}" != "0" ]]; then
  devbox warp restore
fi
`, interactive), nil
}
