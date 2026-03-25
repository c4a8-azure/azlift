package bootstrap

import "testing"

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

func TestRoleGUIDForRole(t *testing.T) {
	if roleGUIDForRole("plan") != "acdd72a7-3385-48ef-bd42-f606fba81ae7" {
		t.Error("plan should map to Reader GUID")
	}
	if roleGUIDForRole("apply") != "b24988ac-6180-42a0-ab88-20f7382dd24c" {
		t.Error("apply should map to Contributor GUID")
	}
	if roleGUIDForRole("unknown") != "acdd72a7-3385-48ef-bd42-f606fba81ae7" {
		t.Error("unknown role should default to Reader GUID")
	}
}
