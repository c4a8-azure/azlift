package refine

import (
	"context"
	"errors"
	"testing"
)

type mockDocs struct {
	output string
	err    error
}

func (m *mockDocs) Run(_ context.Context, _ string) (string, error) {
	return m.output, m.err
}

func TestRunDocs_SkipReturnsEarly(t *testing.T) {
	runner := &mockDocs{}
	res, err := RunDocs(context.Background(), runner, "/tmp", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestRunDocs_SuccessfulRun(t *testing.T) {
	runner := &mockDocs{output: "# Module\n\n## Inputs\n"}
	res, err := RunDocs(context.Background(), runner, "/tmp", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Skipped {
		t.Error("should not be skipped")
	}
	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunDocs_RunnerErrorPropagated(t *testing.T) {
	runner := &mockDocs{err: errors.New("binary not found")}
	_, err := RunDocs(context.Background(), runner, "/tmp", false)
	if err == nil {
		t.Error("expected error when runner fails")
	}
}
