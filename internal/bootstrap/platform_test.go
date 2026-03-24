package bootstrap

import (
	"context"
	"testing"
)

func testPlatformConfig(platform string) PlatformConfig {
	return PlatformConfig{
		Platform:     platform,
		Org:          "my-org",
		RepoName:     "infra-prod",
		Environments: []string{"prod", "staging", "dev"},
		Location:     "westeurope",
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
	// 1 Invoke-AzBootstrap + 2 Add-AzBootstrapEnvironment = 3 calls
	if len(m.Calls) != 3 {
		t.Fatalf("want 3 calls (1 invoke + 2 add), got %d", len(m.Calls))
	}
	if m.Calls[0][0] != "Invoke-AzBootstrap" {
		t.Errorf("first call: want Invoke-AzBootstrap, got %s", m.Calls[0][0])
	}
}

func TestProvisionPlatform_ADO(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("ado")

	err := ProvisionPlatform(context.Background(), m, cfg, nil)
	if err == nil {
		t.Error("expected error: ado not yet supported")
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

	// First call (Invoke-AzBootstrap) must include state storage names.
	args := m.Calls[0]
	assertArg(t, args, "-ResourceGroupName", "rg-tfstate-infra")
	assertArg(t, args, "-TerraformStateStorageAccountName", "stinfraprod")
}

func TestProvisionPlatform_PassesMINames(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := m.Calls[0]
	// Invoke-AzBootstrap should have MI names for the first environment (prod).
	assertArg(t, args, "-PlanManagedIdentityName", "mi-infra-prod-prod-plan")
	assertArg(t, args, "-ApplyManagedIdentityName", "mi-infra-prod-prod-apply")
}

func TestProvisionPlatform_AddsSubsequentEnvironments(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Call 1: Invoke-AzBootstrap (prod)
	// Call 2: Add-AzBootstrapEnvironment (staging)
	// Call 3: Add-AzBootstrapEnvironment (dev)
	if m.Calls[1][0] != "Add-AzBootstrapEnvironment" {
		t.Errorf("call 2: want Add-AzBootstrapEnvironment, got %s", m.Calls[1][0])
	}
	assertArg(t, m.Calls[1], "-EnvironmentName", "staging")
	assertArg(t, m.Calls[2], "-EnvironmentName", "dev")
}

func TestProvisionPlatform_RunnerError(t *testing.T) {
	m := &MockRunner{Err: &ExitError{Code: 1, Stderr: "token expired"}}
	cfg := testPlatformConfig("github")
	err := ProvisionPlatform(context.Background(), m, cfg, nil)
	if err == nil {
		t.Error("expected error when runner fails")
	}
}

func TestProvisionPlatform_SingleEnvironment(t *testing.T) {
	m := &MockRunner{}
	cfg := testPlatformConfig("github")
	cfg.Environments = []string{"prod"}

	if err := ProvisionPlatform(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only 1 call — no Add-AzBootstrapEnvironment needed.
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(m.Calls))
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
