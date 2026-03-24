// Package bootstrap implements Stage 4 of the azlift pipeline: provisioning
// state storage, Managed Identities with OIDC federated credentials, and the
// Git repository with CI/CD pipelines via az-bootstrap (PowerShell 7).
package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner invokes az-bootstrap and streams its output.
// The interface makes it substitutable with a fake in unit tests.
type Runner interface {
	// Run executes az-bootstrap with the given arguments, streaming each
	// output line to logLine. Returns a non-nil error on non-zero exit.
	Run(ctx context.Context, args []string, logLine func(line string)) error
}

// ExitError is returned when az-bootstrap exits with a non-zero code.
type ExitError struct {
	Code   int
	Stderr string // last few lines of stderr
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("az-bootstrap exited %d: %s", e.Code, e.Stderr)
	}
	return fmt.Sprintf("az-bootstrap exited %d", e.Code)
}

// AzBootstrapRunner is the production Runner that invokes az-bootstrap via
// `pwsh -NonInteractive -File <script>`. If ScriptPath is empty it resolves
// az-bootstrap from PATH as a pwsh script named `az-bootstrap.ps1`.
type AzBootstrapRunner struct {
	// ScriptPath is the path to az-bootstrap.ps1.
	// Defaults to "az-bootstrap.ps1" (resolved via PATH).
	ScriptPath string
	// PwshBinary is the PowerShell 7 executable.
	// Defaults to "pwsh".
	PwshBinary string
}

// Run implements Runner.
func (r *AzBootstrapRunner) Run(ctx context.Context, args []string, logLine func(line string)) error {
	pwsh := r.PwshBinary
	if pwsh == "" {
		pwsh = "pwsh"
	}
	script := r.ScriptPath
	if script == "" {
		script = "az-bootstrap.ps1"
	}

	// Build: pwsh -NonInteractive -File <script> <args...>
	pwshArgs := append([]string{"-NonInteractive", "-File", script}, args...)
	cmd := exec.CommandContext(ctx, pwsh, pwshArgs...) //nolint:gosec // pwsh and script are validated tool paths

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("attaching stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("attaching stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting az-bootstrap: %w", err)
	}

	// Stream stdout and stderr concurrently to avoid pipe-buffer deadlock.
	type result struct{ tail string }
	outDone := make(chan result, 1)
	errDone := make(chan result, 1)

	go func() { outDone <- result{streamLines(stdout, logLine)} }()
	go func() { errDone <- result{streamLines(stderr, logLine)} }()

	<-outDone
	stderrTail := (<-errDone).tail

	if err := cmd.Wait(); err != nil {
		code := -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return &ExitError{Code: code, Stderr: stderrTail}
	}
	return nil
}

// MockRunner is a test double for Runner.
type MockRunner struct {
	// Lines are fed to logLine one by one when Run is called.
	Lines []string
	// Err is the error returned by Run (nil = success).
	Err error
	// Calls records all args slices passed to Run.
	Calls [][]string
}

// Run implements Runner.
func (m *MockRunner) Run(_ context.Context, args []string, logLine func(string)) error {
	m.Calls = append(m.Calls, args)
	for _, l := range m.Lines {
		if logLine != nil {
			logLine(l)
		}
	}
	return m.Err
}

// streamLines reads r line by line, calls logLine for each, and returns the
// last 5 lines joined (used for error context). Blocks until EOF.
func streamLines(r io.Reader, logLine func(string)) string {
	var tail []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if logLine != nil {
			logLine(line)
		}
		tail = append(tail, line)
		if len(tail) > 5 {
			tail = tail[1:]
		}
	}
	return strings.Join(tail, "; ")
}
