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

// AzBootstrapRunner is the production Runner that invokes the az-bootstrap
// PowerShell module via `pwsh -NonInteractive -Command`.
//
// args[0] must be the PowerShell cmdlet name ("Invoke-AzBootstrap" or
// "Add-AzBootstrapEnvironment"). Subsequent args are alternating
// "-ParamName", "value" pairs, or bare "-SwitchName" entries.
//
// The runner builds:
//
//	Import-Module az-bootstrap -ErrorAction Stop; <cmdlet> -Param1 'val1' -Switch
type AzBootstrapRunner struct {
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

	psCmd := buildPSCommand(args)
	pwshArgs := []string{"-NonInteractive", "-Command", psCmd}
	cmd := exec.CommandContext(ctx, pwsh, pwshArgs...) //nolint:gosec // pwsh is a validated tool path

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

// buildPSCommand assembles a PowerShell command string from args.
// args[0] is the cmdlet name; remaining args are parameters.
// String values are single-quoted; switch args (starting with '-' and having
// no following value, or followed by another flag) are emitted bare.
func buildPSCommand(args []string) string {
	if len(args) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Import-Module az-bootstrap -ErrorAction Stop; ")
	sb.WriteString(args[0])

	i := 1
	for i < len(args) {
		a := args[i]
		nextIdx := i + 1

		// Check if this looks like a flag name (starts with -)
		if len(a) > 0 && a[0] == '-' {
			// Peek at next arg: if absent or also a flag, treat as a switch.
			nextIsFlag := nextIdx >= len(args) || (len(args[nextIdx]) > 0 && args[nextIdx][0] == '-') //nolint:gosec // nextIdx bound checked on left side of ||
			if nextIsFlag {
				sb.WriteByte(' ')
				sb.WriteString(a)
				i++
			} else {
				// -ParamName 'value'
				val := args[nextIdx] //nolint:gosec // nextIsFlag==false guarantees nextIdx < len(args)
				sb.WriteByte(' ')
				sb.WriteString(a)
				sb.WriteString(" '")
				sb.WriteString(strings.ReplaceAll(val, "'", "''")) // escape single quotes
				sb.WriteByte('\'')
				i += 2
			}
		} else {
			// bare positional value — single-quote it
			sb.WriteString(" '")
			sb.WriteString(strings.ReplaceAll(a, "'", "''"))
			sb.WriteByte('\'')
			i++
		}
	}
	return sb.String()
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
