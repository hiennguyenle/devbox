package tmux

import "fmt"

func NewAttachSession(name string) string {
	return fmt.Sprintf("tmux new -A -s %s", name)
}

func ListSessions() string {
	return "tmux list-sessions"
}

func KillSession(name string) string {
	return fmt.Sprintf("tmux kill-session -t %s", name)
}

func RenameSession(oldName, newName string) string {
	return fmt.Sprintf("tmux rename-session -t %s %s", oldName, newName)
}

func ListWindows(name string) string {
	return fmt.Sprintf("tmux list-windows -t %s", name)
}

func HasSession(name string) string {
	return fmt.Sprintf("tmux has-session -t %s", name)
}
