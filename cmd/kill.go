package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hien/devbox/internal/tmux"
	"github.com/hien/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill [NAME]",
	Short: "Kill a tmux session (interactive picker if no name given)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runKill,
}

func init() {
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}
	client := newClient(cfg)

	// Direct kill by name
	if len(args) == 1 {
		name := args[0]
		if err := validate.SessionName(name); err != nil {
			return err
		}
		return killSession(client, cfg.Host, name)
	}

	// Interactive mode
	output, err := client.Run(tmux.ListSessions())
	if err != nil || strings.TrimSpace(output) == "" {
		fmt.Printf("No active sessions on %s.\n", cfg.Host)
		return nil
	}

	sessions := parseSessions(output)
	if len(sessions) == 0 {
		fmt.Printf("No active sessions on %s.\n", cfg.Host)
		return nil
	}

	model := killPickerModel{sessions: sessions, host: cfg.Host}
	p := tea.NewProgram(model)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker error: %w", err)
	}

	final, ok := result.(killPickerModel)
	if !ok {
		return fmt.Errorf("unexpected picker state")
	}

	for _, name := range final.killed {
		fmt.Printf("Killed %q\n", name)
	}
	return nil
}

func killSession(client interface{ Run(string) (string, error) }, host, name string) error {
	output, err := client.Run(tmux.KillSession(name))
	if err != nil {
		if output != "" {
			return fmt.Errorf("kill session %q: %s", name, output)
		}
		return handleSSHError(err, host)
	}
	fmt.Printf("Session %q killed.\n", name)
	return nil
}

// killPickerModel is the bubbletea model for interactive kill.
type killPickerModel struct {
	sessions []sessionItem
	cursor   int
	killed   []string
	host     string
	quitting bool
	status   string // status message after an action
}

func (m killPickerModel) Init() tea.Cmd {
	return nil
}

func (m killPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
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
		case "x":
			if len(m.sessions) > 0 {
				m = m.killCurrent()
			}
		case "enter":
			if len(m.sessions) > 0 {
				m = m.killCurrent()
				m.quitting = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m killPickerModel) killCurrent() killPickerModel {
	name := m.sessions[m.cursor].name

	// Kill via SSH
	cfg, err := requireConfig()
	if err != nil {
		m.status = fmt.Sprintf("Error: %v", err)
		return m
	}
	client := newClient(cfg)
	_, err = client.Run(tmux.KillSession(name))
	if err != nil {
		m.status = fmt.Sprintf("Failed to kill %q", name)
		return m
	}

	m.killed = append(m.killed, name)
	m.sessions = append(m.sessions[:m.cursor], m.sessions[m.cursor+1:]...)
	if m.cursor >= len(m.sessions) && m.cursor > 0 {
		m.cursor--
	}
	m.status = fmt.Sprintf("Killed %q", name)

	if len(m.sessions) == 0 {
		m.quitting = true
	}
	return m
}

var (
	killSelectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	killUnselectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	killHeaderStyle     = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	killStatusStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func (m killPickerModel) View() string {
	if m.quitting && len(m.sessions) == 0 {
		return "All sessions killed.\n"
	}
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(killHeaderStyle.Render("Select sessions to kill:"))
	b.WriteString("\n\n")

	for i, s := range m.sessions {
		cursor := "  "
		style := killUnselectedStyle
		if m.cursor == i {
			cursor = "> "
			style = killSelectedStyle
		}
		line := fmt.Sprintf("%s%s", cursor, s.name)
		if s.details != "" {
			line += fmt.Sprintf("  %s", s.details)
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(killStatusStyle.Render(m.status))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(killUnselectedStyle.Render("↑/↓ navigate • x kill • enter kill & exit • esc exit"))
	return b.String()
}
