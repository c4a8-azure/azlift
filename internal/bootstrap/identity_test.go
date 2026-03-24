package bootstrap

import (
	"context"
	"testing"
)

func TestMIName_Basic(t *testing.T) {
	name := MIName("infra-prod", "prod", "plan")
	if name != "mi-infra-prod-prod-plan" {
		t.Errorf("unexpected MI name: %s", name)
	}
}

func TestMIName_SpecialChars(t *testing.T) {
	name := MIName("My_Infra.Repo", "staging", "apply")
	// Slug: my-infra-repo
	if name != "mi-my-infra-repo-staging-apply" {
		t.Errorf("unexpected MI name: %s", name)
	}
}

func TestRbacRoleForRole(t *testing.T) {
	if rbacRoleForRole("plan") != "Reader" {
		t.Error("plan should map to Reader")
	}
	if rbacRoleForRole("apply") != "Contributor" {
		t.Error("apply should map to Contributor")
	}
	if rbacRoleForRole("unknown") != "Reader" {
		t.Error("unknown role should default to Reader")
	}
}

func TestProvisionIdentities_CallsRunnerPerEnvRole(t *testing.T) {
	m := &MockRunner{}
	oidc := OIDCConfig{
		Platform: "github",
		RepoOrg:  "my-org",
		RepoName: "infra-prod",
	}

	result, err := ProvisionIdentities(
		context.Background(), m,
		"sub-123", "rg-mi",
		"infra-prod",
		[]string{"prod", "staging", "dev"},
		oidc, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 3 environments × 2 roles = 6 calls
	if len(m.Calls) != 6 {
		t.Errorf("want 6 runner calls, got %d", len(m.Calls))
	}

	// All calls should start with "managed-identity"
	for i, call := range m.Calls {
		if call[0] != "managed-identity" {
			t.Errorf("call %d: first arg should be managed-identity, got %s", i, call[0])
		}
	}

	// Result should have 6 identities.
	if len(result.Identities) != 6 {
		t.Errorf("want 6 identities in result, got %d", len(result.Identities))
	}
}

func TestProvisionIdentities_OIDCEnvironmentNaming(t *testing.T) {
	m := &MockRunner{}
	oidc := OIDCConfig{Platform: "github", RepoOrg: "org", RepoName: "repo"}

	_, err := ProvisionIdentities(
		context.Background(), m,
		"sub-123", "rg-mi",
		"repo",
		[]string{"prod"},
		oidc, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find --oidc-environment args across all calls.
	oidcEnvs := map[string]bool{}
	for _, call := range m.Calls {
		for i, arg := range call {
			if arg == "--oidc-environment" && i+1 < len(call) {
				oidcEnvs[call[i+1]] = true
			}
		}
	}

	if !oidcEnvs["prod-iac-plan"] {
		t.Error("expected prod-iac-plan OIDC environment")
	}
	if !oidcEnvs["prod-iac-apply"] {
		t.Error("expected prod-iac-apply OIDC environment")
	}
}

func TestProvisionIdentities_RunnerErrorStops(t *testing.T) {
	m := &MockRunner{Err: &ExitError{Code: 1, Stderr: "permission denied"}}
	oidc := OIDCConfig{Platform: "github", RepoOrg: "org", RepoName: "repo"}

	_, err := ProvisionIdentities(
		context.Background(), m,
		"sub-123", "rg-mi",
		"repo",
		[]string{"prod"},
		oidc, nil,
	)
	if err == nil {
		t.Error("expected error when runner fails")
	}
}
