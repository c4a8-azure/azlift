package export

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMockRunner_RecordsCalls(t *testing.T) {
	m := &MockRunner{}
	args := []string{"resource-group", "--resource-group", "rg-test", "--output-dir", "./raw"}
	if err := m.Run(context.Background(), args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0][0] != "resource-group" {
		t.Errorf("first arg: want resource-group, got %s", m.Calls[0][0])
	}
}

func TestMockRunner_PropagatesError(t *testing.T) {
	m := &MockRunner{Err: errors.New("auth failure")}
	err := m.Run(context.Background(), nil, nil)
	if err == nil || err.Error() != "auth failure" {
		t.Errorf("expected auth failure, got %v", err)
	}
}

func TestMockRunner_FeedsLinesToLogLine(t *testing.T) {
	m := &MockRunner{Lines: []string{"Exporting resource...", "Done."}}
	var captured []string
	_ = m.Run(context.Background(), nil, func(line string) {
		captured = append(captured, line)
	})
	if len(captured) != 2 {
		t.Fatalf("want 2 lines, got %d", len(captured))
	}
	if captured[0] != "Exporting resource..." {
		t.Errorf("unexpected first line: %s", captured[0])
	}
}

func TestExitError_Message(t *testing.T) {
	e := &ExitError{Code: 1, Stderr: "resource not found"}
	if !strings.Contains(e.Error(), "1") {
		t.Error("error message should contain exit code")
	}
	if !strings.Contains(e.Error(), "resource not found") {
		t.Error("error message should contain stderr")
	}
}

func TestExitError_NoStderr(t *testing.T) {
	e := &ExitError{Code: 2}
	if !strings.Contains(e.Error(), "2") {
		t.Error("error message should contain exit code")
	}
}

func TestStreamLines_CollectsTail(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5\nline6\n"
	var logged []string
	tail := streamLines(strings.NewReader(input), func(l string) {
		logged = append(logged, l)
	})
	if len(logged) != 6 {
		t.Errorf("want 6 logged lines, got %d", len(logged))
	}
	// tail keeps last 5
	if !strings.Contains(tail, "line2") {
		t.Errorf("tail should contain line2 onwards, got: %s", tail)
	}
	if strings.Contains(tail, "line1") {
		t.Errorf("tail should have dropped line1, got: %s", tail)
	}
}
