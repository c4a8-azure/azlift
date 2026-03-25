// Package export implements Stage 2 of the azlift pipeline: wrapping aztfexport
// with retry logic, exclusion lists, and unsupported-resource data source mapping.
package export

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner executes aztfexport and streams its output. The interface makes it
// trivially substitutable with a fake in unit tests.
type Runner interface {
	// Run invokes aztfexport with the given arguments, streaming output to
	// logLine as each line arrives. Returns a non-nil error on non-zero exit.
	Run(ctx context.Context, args []string, logLine func(line string)) error
}

// ExitError is returned when aztfexport exits with a non-zero code.
type ExitError struct {
	Code   int
	Stderr string // last few lines of stderr output
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("aztfexport exited %d: %s", e.Code, e.Stderr)
	}
	return fmt.Sprintf("aztfexport exited %d", e.Code)
}

// AztfexportRunner is the production Runner that shells out to the real binary.
type AztfexportRunner struct {
	// Binary is the path or name of the aztfexport executable.
	// Defaults to "aztfexport" (resolved via PATH).
	Binary string
	// ExtraEnv is a list of "KEY=VALUE" pairs appended to the process environment.
	// Use this to inject ARM_USE_AZUREAD=true etc. without mutating os.Environ.
	ExtraEnv []string
}

// Run implements Runner.
func (r *AztfexportRunner) Run(ctx context.Context, args []string, logLine func(line string)) error {
	bin := r.Binary
	if bin == "" {
		bin = "aztfexport"
	}

	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // binary is a validated tool path
	if len(r.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), r.ExtraEnv...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("attaching stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("attaching stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting aztfexport: %w", err)
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

// streamLines reads lines from r, calls logLine for each, and returns the
// last ~5 lines joined (useful for error context). It blocks until EOF.
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
