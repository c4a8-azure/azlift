package export

import "context"

// MockRunner is a test double for Runner. Tests set Err to simulate failures
// and inspect Calls to verify the arguments passed to aztfexport.
type MockRunner struct {
	// Err is returned by Run when non-nil.
	Err error
	// Lines are fed to logLine to simulate aztfexport output.
	Lines []string
	// Calls records every (args) slice passed to Run.
	Calls [][]string
}

// Run implements Runner.
func (m *MockRunner) Run(_ context.Context, args []string, logLine func(string)) error {
	m.Calls = append(m.Calls, args)
	for _, line := range m.Lines {
		if logLine != nil {
			logLine(line)
		}
	}
	return m.Err
}
