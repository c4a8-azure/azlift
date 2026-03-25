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

func TestRunLint_ErrorLevelFails(t *testing.T) {
	out := "Error: Missing required attribute (azurerm_virtual_network_required_attrs)\n"
	runner := &mockTflint{output: out, exitCode: 1}
	_, err := RunLint(context.Background(), runner, "/tmp", false)
	if err == nil {
		t.Error("expected error for tflint Error-level finding")
	}
}

func TestRunLint_WarningOnlyIsNonFatal(t *testing.T) {
	out := "Warning: terraform required_version attribute is required (terraform_required_version)\n"
	runner := &mockTflint{output: out, exitCode: 1}
	res, err := RunLint(context.Background(), runner, "/tmp", false)
	if err != nil {
		t.Errorf("warnings should be non-fatal, got error: %v", err)
	}
	if res.Issues != 1 {
		t.Errorf("want Issues=1, got %d", res.Issues)
	}
}

func TestRunLint_ErrorContainsOutput(t *testing.T) {
	out := "Error: Deprecated resource type (some_rule)\n"
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
Error: Missing required attribute (rule1)
Warning: Resource type deprecated (rule2)
Some other line
`
	errs, warns := countIssues(output)
	if errs != 1 {
		t.Errorf("want 1 error, got %d", errs)
	}
	if warns != 1 {
		t.Errorf("want 1 warning, got %d", warns)
	}
}

func TestCountIssues_Clean(t *testing.T) {
	errs, warns := countIssues("1 issue(s) found:\n")
	if errs != 0 || warns != 0 {
		t.Errorf("want 0 errors and 0 warnings, got %d/%d", errs, warns)
	}
}
