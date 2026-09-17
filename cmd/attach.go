package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
	"github.com/hiennguyenle/devbox/internal/tmux"
)

// No attachCmd — picker is accessed via "devbox resume" (no args).

type sessionItem struct {
	name    string
	details string
}

type pickerModel struct {
	sessions []sessionItem
	cursor   int
	chosen   string
	quitting bool
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.sessions[m.cursor].name
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	unselectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	headerStyle     = lipgloss.NewStyle().Bold(true).MarginBottom(1)
)

func (m pickerModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Select a session:"))
	b.WriteString("\n\n")

	for i, s := range m.sessions {
		cursor := "  "
		style := unselectedStyle
		if m.cursor == i {
			cursor = "> "
			style = selectedStyle
		}
		line := fmt.Sprintf("%s%s", cursor, s.name)
		if s.details != "" {
			line += fmt.Sprintf("  %s", s.details)
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(unselectedStyle.Render("↑/↓ navigate • enter attach • esc exit"))
	return b.String()
}

func parseSessions(output string) []sessionItem {
	var items []sessionItem
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// tmux list-sessions format: "name: N windows (created ...)"
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		details := ""
		if len(parts) > 1 {
			details = strings.TrimSpace(parts[1])
		}
		items = append(items, sessionItem{name: name, details: details})
	}
	return items
}

// isNoServerOutput reports whether a failed list-sessions just means "no
// sessions yet". tmux prints "no server running" after its server has exited,
// but "error connecting to <socket> (No such file or directory)" on a box
// where the server never started at all.
func isNoServerOutput(output string) bool {
	return strings.Contains(output, "no server running") ||
		strings.Contains(output, "error connecting to") ||
		strings.Contains(output, "no sessions")
}

// listSessionsForPicker fetches remote sessions for the interactive pickers.
// "no sessions yet" returns an empty slice; a transport failure returns an
// error instead of masquerading as an empty list.
func listSessionsForPicker(client sshpkg.Executor, host string, local bool) ([]sessionItem, error) {
	output, err := client.Run(tmux.ListSessions())
	if err != nil {
		if isNoServerOutput(output) {
			return nil, nil
		}
		return nil, handleSSHError(output, err, host, local)
	}
	return parseSessions(output), nil
}

// runAttachPicker shows an interactive session picker for a resolved profile.
// preSelect is the session name to highlight initially (e.g. the terminal's
// last session on this profile), or "" for default.
func runAttachPicker(profName, host, defaultSession string, client sshpkg.Executor, preSelect string) error {
	local := profName == localProfileName
	sessions, err := listSessionsForPicker(client, host, local)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions available. Creating default session...")
		recordProfileSession(profName, defaultSession)
		return execTmuxSession(client, profName, host, defaultSession)
	}

	// Pre-select the cursor to the last used session if known
	cursor := 0
	if preSelect != "" {
		for i, s := range sessions {
			if s.name == preSelect {
				cursor = i
				break
			}
		}
	}

	model := pickerModel{sessions: sessions, cursor: cursor}
	p := tea.NewProgram(model)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	final, ok := result.(pickerModel)
	if !ok {
		return fmt.Errorf("unexpected picker state")
	}
	if final.quitting || final.chosen == "" {
		return nil // user cancelled
	}

	recordProfileSession(profName, final.chosen)
	return execTmuxSession(client, profName, host, final.chosen)
}
