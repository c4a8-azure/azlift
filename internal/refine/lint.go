package refine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// LintResult captures the outcome of a tflint run.
type LintResult struct {
	// Output is the combined stdout+stderr from tflint.
	Output string
	// Issues is the number of issues reported (0 = clean).
	Issues int
	// Skipped is true when lint was bypassed via --skip-lint.
	Skipped bool
}

// TflintRunner abstracts running tflint so it can be replaced in tests.
type TflintRunner interface {
	Run(ctx context.Context, dir string) (output string, exitCode int, err error)
}

// ExecTflintRunner runs the real tflint binary.
type ExecTflintRunner struct{}

// Run executes tflint in dir and returns combined output and exit code.
func (r *ExecTflintRunner) Run(ctx context.Context, dir string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "tflint", "--chdir", dir) //nolint:gosec // dir is our own output directory
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return output, exitErr.ExitCode(), nil
		}
		return output, -1, fmt.Errorf("running tflint: %w", err)
	}
	return output, 0, nil
}

// RunLint runs tflint against dir using runner. When skip is true it returns
// a Skipped result without invoking the runner.
// Warnings are non-fatal: they are captured in LintResult but do not cause
// an error return. Only tflint Error-level findings fail the pipeline.
func RunLint(ctx context.Context, runner TflintRunner, dir string, skip bool) (LintResult, error) {
	if skip {
		return LintResult{Skipped: true}, nil
	}

	output, exitCode, err := runner.Run(ctx, dir)
	if err != nil {
		return LintResult{Output: output}, err
	}

	errors, warnings := countIssues(output)
	result := LintResult{Output: output, Issues: errors + warnings}

	if exitCode != 0 && errors > 0 {
		return result, fmt.Errorf("tflint reported %d error(s):\n%s", errors, output)
	}
	return result, nil
}

// countIssues counts tflint findings by severity.
// tflint outputs one "Error:" or "Warning:" line per finding.
func countIssues(output string) (errors, warnings int) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Error:"):
			errors++
		case strings.HasPrefix(line, "Warning:"):
			warnings++
		}
	}
	return
}
