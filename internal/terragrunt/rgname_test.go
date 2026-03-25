package terragrunt

import (
	"testing"
)

func TestSubstituteEnv_SuffixReplacement(t *testing.T) {
	got := substituteEnv("rg-myapp-prod", "prod", "dev")
	if got != "rg-myapp-dev" {
		t.Errorf("want rg-myapp-dev, got %s", got)
	}
}

func TestSubstituteEnv_SameEnv(t *testing.T) {
	got := substituteEnv("rg-myapp-prod", "prod", "prod")
	if got != "rg-myapp-prod" {
		t.Errorf("want unchanged rg-myapp-prod, got %s", got)
	}
}

func TestSubstituteEnv_NoMatch_AppendsEnv(t *testing.T) {
	got := substituteEnv("rg-myapp", "prod", "dev")
	if got != "rg-myapp-dev" {
		t.Errorf("want rg-myapp-dev, got %s", got)
	}
}

func TestSubstituteEnv_SubstringFallback(t *testing.T) {
	got := substituteEnv("prod-rg-myapp", "prod", "staging")
	if got != "staging-rg-myapp" {
		t.Errorf("want staging-rg-myapp, got %s", got)
	}
}

func TestDeriveEnvRGInputs_MultipleRGs(t *testing.T) {
	rgLocals := map[string]string{
		"resource_group_name_001": "rg-net-prod",
		"resource_group_name_002": "rg-app-prod",
	}
	out := deriveEnvRGInputs(rgLocals, "rg-net-prod", "prod", "dev")
	if out["resource_group_name_001"] != "rg-net-dev" {
		t.Errorf("want rg-net-dev, got %s", out["resource_group_name_001"])
	}
	if out["resource_group_name_002"] != "rg-app-dev" {
		t.Errorf("want rg-app-dev, got %s", out["resource_group_name_002"])
	}
}
