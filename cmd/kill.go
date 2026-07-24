package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hiennguyenle/devbox/internal/config"
	sshpkg "github.com/hiennguyenle/devbox/internal/ssh"
	"github.com/hiennguyenle/devbox/internal/tmux"
	"github.com/hiennguyenle/devbox/internal/validate"
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
	sessions, err := listSessionsForPicker(client, cfg.Host)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Printf("No active sessions on %s.\n", cfg.Host)
		return nil
	}

	model := killPickerModel{client: client, host: cfg.Host, sessions: sessions}
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
	if final.failed != "" {
		return fmt.Errorf("failed to kill %q", final.failed)
	}
	return nil
}

func killSession(client sshpkg.Executor, host, name string) error {
	output, err := client.Run(tmux.KillSession(name))
	if err != nil {
		if output != "" {
			return fmt.Errorf("kill session %q: %s", name, output)
		}
		return handleSSHError(output, err, host)
	}
	config.DeleteSessionsByName(config.SessionsDir(), name)
	fmt.Printf("Killed %q\n", name)
	return nil
}

// killResultMsg is delivered back to the picker when an async kill finishes.
type killResultMsg struct {
	name      string
	err       error
	quitAfter bool
}

// killPickerModel is the bubbletea model for interactive kill. Kills run as
// tea.Cmds so a slow SSH round-trip never freezes the UI.
type killPickerModel struct {
	client      sshpkg.Executor
	host        string
	sessions    []sessionItem
	cursor      int
	killed      []string
	failed      string // last session a kill failed for, surfaced after quit
	quitting    bool
	inflight    bool
	quitPending bool   // user asked to quit while a kill was in flight
	status      string // status message after an action
}

func (m killPickerModel) Init() tea.Cmd {
	return nil
}

func (m killPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inflight {
			// One kill at a time. Quit requests are deferred until the
			// result arrives: the remote kill already ran, so cleanup and
			// reporting must still happen.
			if s := msg.String(); s == "ctrl+c" || s == "esc" {
				m.quitPending = true
				m.status = "Finishing..."
			}
			return m, nil
		}
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
				return m.startKill(false)
			}
		case "enter":
			if len(m.sessions) > 0 {
				return m.startKill(true)
			}
		}
	case killResultMsg:
		return m.finishKill(msg)
	}
	return m, nil
}

// startKill dispatches the SSH kill for the session under the cursor as an
// async tea.Cmd.
func (m killPickerModel) startKill(quitAfter bool) (tea.Model, tea.Cmd) {
	name := m.sessions[m.cursor].name
	m.inflight = true
	m.status = fmt.Sprintf("Killing %q...", name)
	client := m.client
	return m, func() tea.Msg {
		_, err := client.Run(tmux.KillSession(name))
		return killResultMsg{name: name, err: err, quitAfter: quitAfter}
	}
}

func (m killPickerModel) finishKill(msg killResultMsg) (tea.Model, tea.Cmd) {
	m.inflight = false
	if msg.err != nil {
		m.status = fmt.Sprintf("Failed to kill %q", msg.name)
		m.failed = msg.name
		if msg.quitAfter || m.quitPending {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	config.DeleteSessionsByName(config.SessionsDir(), msg.name)
	m.killed = append(m.killed, msg.name)
	for i, s := range m.sessions {
		if s.name == msg.name {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			break
		}
	}
	if m.cursor >= len(m.sessions) && m.cursor > 0 {
		m.cursor--
	}
	m.status = fmt.Sprintf("Killed %q", msg.name)

	if msg.quitAfter || m.quitPending || len(m.sessions) == 0 {
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
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
