package refine

import (
	"context"
	"strings"
	"testing"
)

// mockTflint is a test double for TflintRunner.
type mockTflint struct {
	output   string
	exitCode int
	err      error
}

func (m *mockTflint) Run(_ context.Context, _ string) (string, int, error) {
	return m.output, m.exitCode, m.err
}

func TestRunLint_SkipReturnsEarly(t *testing.T) {
	runner := &mockTflint{}
	res, err := RunLint(context.Background(), runner, "/tmp", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestRunLint_CleanRun(t *testing.T) {
	runner := &mockTflint{output: "", exitCode: 0}
	res, err := RunLint(context.Background(), runner, "/tmp", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Issues != 0 {
		t.Errorf("want 0 issues, got %d", res.Issues)
	}
	if res.Skipped {
		t.Error("should not be skipped")
	}
}

func TestRunLint_NonZeroExitReturnsError(t *testing.T) {
	out := "[error] Missing required attribute (azurerm_virtual_network_required_attrs)\n"
	runner := &mockTflint{output: out, exitCode: 2}
	_, err := RunLint(context.Background(), runner, "/tmp", false)
	if err == nil {
		t.Error("expected error for non-zero exit")
	}
}

func TestRunLint_ErrorContainsOutput(t *testing.T) {
	out := "[warning] Deprecated resource type (aws_deprecated)\n"
	runner := &mockTflint{output: out, exitCode: 1}
	_, err := RunLint(context.Background(), runner, "/tmp", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Deprecated") {
		t.Errorf("error should contain tflint output, got: %v", err)
	}
}

func TestCountIssues_MultipleFindings(t *testing.T) {
	output := `
[error] Missing required attribute (rule1)
[warning] Resource type deprecated (rule2)
[notice] Use newer syntax (rule3)
Some other line
`
	count := countIssues(output)
	if count != 3 {
		t.Errorf("want 3 issues, got %d", count)
	}
}

func TestCountIssues_Clean(t *testing.T) {
	count := countIssues("tflint: No issues found.\n")
	if count != 0 {
		t.Errorf("want 0 issues, got %d", count)
	}
}
