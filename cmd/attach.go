package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hien/devbox/internal/config"
	sshpkg "github.com/hien/devbox/internal/ssh"
	"github.com/hien/devbox/internal/tmux"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach",
	Short: "Interactive session picker",
	RunE:  runAttach,
}

func init() {
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	client := newClient(cfg)
	return runAttachPicker(cfg, client)
}

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
	b.WriteString(unselectedStyle.Render("↑/↓ navigate • enter select • q quit"))
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

// runAttachPicker is shared by resume, last, and attach commands.
// It fetches sessions, shows the bubbletea picker, and attaches to the chosen session.
func runAttachPicker(cfg *config.Config, client *sshpkg.Client) error {
	output, err := client.Run(tmux.ListSessions())
	if err != nil || strings.TrimSpace(output) == "" {
		fmt.Println("No sessions available. Creating default session...")
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	sessions := parseSessions(output)
	if len(sessions) == 0 {
		saveCurrentSession(cfg.DefaultSession)
		return client.Exec(tmux.NewAttachSession(cfg.DefaultSession))
	}

	model := pickerModel{sessions: sessions}
	p := tea.NewProgram(model)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	final := result.(pickerModel)
	if final.quitting || final.chosen == "" {
		return nil // user cancelled
	}

	saveCurrentSession(final.chosen)
	return client.Exec(tmux.NewAttachSession(final.chosen))
}
