package refine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// DocsResult captures the outcome of a terraform-docs run.
type DocsResult struct {
	// Output is the generated markdown content written to README.md.
	Output string
	// Skipped is true when docs generation was bypassed via --skip-docs.
	Skipped bool
}

// TerraformDocsRunner abstracts running terraform-docs so tests can inject
// a double without requiring the binary to be installed.
type TerraformDocsRunner interface {
	Run(ctx context.Context, dir string) (output string, err error)
}

// ExecTerraformDocsRunner runs the real terraform-docs binary.
type ExecTerraformDocsRunner struct{}

// Run executes `terraform-docs markdown table --output-file README.md <dir>`.
func (r *ExecTerraformDocsRunner) Run(ctx context.Context, dir string) (string, error) {
	//nolint:gosec // dir is our own output directory
	cmd := exec.CommandContext(ctx,
		"terraform-docs",
		"markdown", "table",
		"--output-file", "README.md",
		dir,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("terraform-docs: %w", err)
	}
	return buf.String(), nil
}

// RunDocs generates README.md in dir using runner. When skip is true it
// returns a Skipped result immediately without invoking the runner.
func RunDocs(ctx context.Context, runner TerraformDocsRunner, dir string, skip bool) (DocsResult, error) {
	if skip {
		return DocsResult{Skipped: true}, nil
	}

	output, err := runner.Run(ctx, dir)
	if err != nil {
		return DocsResult{Output: output}, err
	}
	return DocsResult{Output: output}, nil
}
