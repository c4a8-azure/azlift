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
