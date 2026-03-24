package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommitMessage_ContainsSubscription(t *testing.T) {
	msg := buildCommitMessage("sub-abc-123")
	if !strings.Contains(msg, "sub-abc-123") {
		t.Errorf("commit message should contain subscription ID, got: %s", msg)
	}
}

func TestBuildCommitMessage_ContainsVersion(t *testing.T) {
	msg := buildCommitMessage("sub-123")
	if !strings.Contains(msg, "azlift") {
		t.Errorf("commit message should reference azlift, got: %s", msg)
	}
}

func TestBuildCommitMessage_StartsWithChore(t *testing.T) {
	msg := buildCommitMessage("sub-123")
	if !strings.HasPrefix(msg, "chore:") {
		t.Errorf("commit message should start with chore:, got: %s", msg)
	}
}

func TestWriteAzBootstrapConfig_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := &AzBootstrapConfig{
		SchemaVersion:  "1",
		SubscriptionID: "sub-123",
		TenantID:       "tenant-456",
	}

	if err := writeAzBootstrapConfig(tmp, cfg); err != nil {
		t.Fatalf("writeAzBootstrapConfig: %v", err)
	}

	dest := filepath.Join(tmp, ".azbootstrap.jsonc")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading .azbootstrap.jsonc: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "sub-123") {
		t.Error("expected subscription ID in .azbootstrap.jsonc")
	}
	if !strings.Contains(out, "tenant-456") {
		t.Error("expected tenant ID in .azbootstrap.jsonc")
	}
}

func TestWriteAzBootstrapConfig_ValidJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := &AzBootstrapConfig{SchemaVersion: "1", SubscriptionID: "sub-123"}

	if err := writeAzBootstrapConfig(tmp, cfg); err != nil {
		t.Fatalf("writeAzBootstrapConfig: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, ".azbootstrap.jsonc"))
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Errorf(".azbootstrap.jsonc is not valid JSON: %v", err)
	}
}
