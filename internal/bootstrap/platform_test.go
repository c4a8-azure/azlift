package bootstrap

import (
	"context"
	"testing"
)

func testPlatformConfig(platform string) PlatformConfig {
	return PlatformConfig{
		Platform:       platform,
		Org:            "my-org",
		RepoName:       "infra-prod",
		Environments:   []string{"prod", "staging", "dev"},
		SubscriptionID: "sub-123",
		TenantID:       "tenant-456",
		Identities: map[string]string{
			"prod/plan":    "client-plan-prod",
			"prod/apply":   "client-apply-prod",
			"staging/plan": "client-plan-staging",
		},
		StateStorage: StateStorageConfig{
			ResourceGroupName:  "rg-tfstate-infra",
			StorageAccountName: "stinfraprod",
			ContainerName:      "tfstate",
		},
	}
}

func TestProvisionPlatform_GitHub(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0][0] != "github" {
		t.Errorf("first arg: want github, got %s", m.Calls[0][0])
	}
}

func TestProvisionPlatform_ADO(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("ado")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Calls[0][0] != "ado" {
		t.Errorf("first arg: want ado, got %s", m.Calls[0][0])
	}
}

func TestProvisionPlatform_UnknownPlatform(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("bitbucket")
	err := ProvisionPlatform(context.Background(), m, cfg, nil)
	if err == nil {
		t.Error("expected error for unsupported platform")
	}
}

func TestProvisionPlatform_PassesStateBackend(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := m.Calls[0]
	assertArg(t, args, "--state-resource-group", "rg-tfstate-infra")
	assertArg(t, args, "--state-storage-account", "stinfraprod")
	assertArg(t, args, "--state-container", "tfstate")
}

func TestProvisionPlatform_PassesClientIDs(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := m.Calls[0]
	found := false
	for _, a := range args {
		if a == "--plan-client-id-prod=client-plan-prod" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --plan-client-id-prod=client-plan-prod in args: %v", args)
	}
}

func TestProvisionPlatform_RunnerError(t *testing.T) {
	m := &MockRunner{Err: &ExitError{Code: 1, Stderr: "token expired"}}
	cfg := testPlatformConfig("github")
	err := ProvisionPlatform(context.Background(), m, cfg, nil)
	if err == nil {
		t.Error("expected error when runner fails")
	}
}

// assertArg checks that flag appears in args followed by want.
func assertArg(t *testing.T, args []string, flag, want string) {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == want {
			return
		}
	}
	t.Errorf("expected %s %s in args: %v", flag, want, args)
}
