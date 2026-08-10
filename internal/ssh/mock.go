package ssh

import "strings"

type MockExecutor struct {
	RunCalls         []string
	ExecCalls        []string
	RunOutput        string
	RunErr           error
	ExecErr          error
	DisconnectOnExec bool
}

func (m *MockExecutor) Run(remoteCmd string) (string, error) {
	m.RunCalls = append(m.RunCalls, remoteCmd)
	return m.RunOutput, m.RunErr
}

func (m *MockExecutor) Exec(remoteCmd string, onDisconnect func()) error {
	m.ExecCalls = append(m.ExecCalls, remoteCmd)
	if m.DisconnectOnExec && onDisconnect != nil {
		onDisconnect()
	}
	return m.ExecErr
}

func (m *MockExecutor) LastRunContains(substr string) bool {
	if len(m.RunCalls) == 0 {
		return false
	}
	return strings.Contains(m.RunCalls[len(m.RunCalls)-1], substr)
}

func (m *MockExecutor) LastExecContains(substr string) bool {
	if len(m.ExecCalls) == 0 {
		return false
	}
	return strings.Contains(m.ExecCalls[len(m.ExecCalls)-1], substr)
}
