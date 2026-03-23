package enrich

import (
	"os"
	"testing"
)

func TestNewClient_MissingKeyError(t *testing.T) {
	orig := os.Getenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", orig)

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error when ANTHROPIC_API_KEY is not set")
	}
	if !containsAny(err.Error(), "ANTHROPIC_API_KEY", "environment variable") {
		t.Errorf("error should mention ANTHROPIC_API_KEY, got: %v", err)
	}
}

func TestNewClient_KeyPresent(t *testing.T) {
	orig := os.Getenv("ANTHROPIC_API_KEY")
	os.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	defer os.Setenv("ANTHROPIC_API_KEY", orig)

	c, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error with key set: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewClientWithModel_SetsModel(t *testing.T) {
	orig := os.Getenv("ANTHROPIC_API_KEY")
	os.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	defer os.Setenv("ANTHROPIC_API_KEY", orig)

	c, err := NewClientWithModel("claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", c.model)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}
