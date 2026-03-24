package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMockRunner_RecordsCalls(t *testing.T) {
	m := &MockRunner{}
	args := []string{"--subscription", "sub-123", "--repo-name", "infra-prod"}
	if err := m.Run(context.Background(), args, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0][0] != "--subscription" {
		t.Errorf("first arg: want --subscription, got %s", m.Calls[0][0])
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
	m := &MockRunner{Lines: []string{"Provisioning state storage...", "Done."}}
	var captured []string
	_ = m.Run(context.Background(), nil, func(line string) {
		captured = append(captured, line)
	})
	if len(captured) != 2 {
		t.Fatalf("want 2 lines, got %d", len(captured))
	}
	if captured[0] != "Provisioning state storage..." {
		t.Errorf("unexpected first line: %s", captured[0])
	}
}

func TestExitError_WithStderr(t *testing.T) {
	e := &ExitError{Code: 1, Stderr: "subscription not found"}
	if !strings.Contains(e.Error(), "1") {
		t.Error("error should contain exit code")
	}
	if !strings.Contains(e.Error(), "subscription not found") {
		t.Error("error should contain stderr")
	}
}

func TestExitError_NoStderr(t *testing.T) {
	e := &ExitError{Code: 2}
	if !strings.Contains(e.Error(), "2") {
		t.Error("error should contain exit code")
	}
}

func TestBuildPSCommand_BasicCmdlet(t *testing.T) {
	cmd := buildPSCommand([]string{"Invoke-AzBootstrap", "-TargetRepoName", "infra-prod", "-GitHubOwner", "my-org"})
	if !strings.Contains(cmd, "Import-Module az-bootstrap") {
		t.Errorf("expected module import in command: %s", cmd)
	}
	if !strings.Contains(cmd, "Invoke-AzBootstrap") {
		t.Errorf("expected cmdlet in command: %s", cmd)
	}
	if !strings.Contains(cmd, "-TargetRepoName 'infra-prod'") {
		t.Errorf("expected param in command: %s", cmd)
	}
}

func TestBuildPSCommand_Switch(t *testing.T) {
	cmd := buildPSCommand([]string{"Invoke-AzBootstrap", "-TargetRepoName", "repo", "-SkipConfirmation"})
	if !strings.Contains(cmd, "-SkipConfirmation") {
		t.Errorf("expected switch in command: %s", cmd)
	}
	// Switch should not be followed by a quoted value
	if strings.Contains(cmd, "-SkipConfirmation '") {
		t.Errorf("switch should not be quoted: %s", cmd)
	}
}

func TestBuildPSCommand_QuotesSingleQuotes(t *testing.T) {
	cmd := buildPSCommand([]string{"Invoke-AzBootstrap", "-Name", "it's-a-repo"})
	if !strings.Contains(cmd, "it''s-a-repo") {
		t.Errorf("single quotes in value should be escaped: %s", cmd)
	}
}

func TestBuildPSCommand_Empty(t *testing.T) {
	cmd := buildPSCommand(nil)
	if cmd != "" {
		t.Errorf("empty args should return empty string, got: %s", cmd)
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
	if strings.Contains(tail, "line1") {
		t.Errorf("tail should have dropped line1, got: %s", tail)
	}
	if !strings.Contains(tail, "line2") {
		t.Errorf("tail should contain line2 onwards, got: %s", tail)
	}
}
