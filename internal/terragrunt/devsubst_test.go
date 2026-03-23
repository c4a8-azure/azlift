package terragrunt

import (
	"testing"
)

func TestApplyDevSubstitutions_GRStoLRS(t *testing.T) {
	inputs := map[string]string{
		"account_replication_type": `"GRS"`,
	}
	out := ApplyDevSubstitutions(inputs, DefaultSubstitutions)
	if out["account_replication_type"] != `"LRS"` {
		t.Errorf("expected LRS, got %s", out["account_replication_type"])
	}
}

func TestApplyDevSubstitutions_PremiumToStandard(t *testing.T) {
	inputs := map[string]string{
		"sku": `"Premium"`,
	}
	out := ApplyDevSubstitutions(inputs, DefaultSubstitutions)
	if out["sku"] != `"Standard"` {
		t.Errorf("expected Standard, got %s", out["sku"])
	}
}

func TestApplyDevSubstitutions_ZoneRedundantFalse(t *testing.T) {
	inputs := map[string]string{
		"zone_redundant": "true",
	}
	out := ApplyDevSubstitutions(inputs, DefaultSubstitutions)
	if out["zone_redundant"] != "false" {
		t.Errorf("expected false, got %s", out["zone_redundant"])
	}
}

func TestApplyDevSubstitutions_NoMatchUnchanged(t *testing.T) {
	inputs := map[string]string{
		"location": `"westeurope"`,
	}
	out := ApplyDevSubstitutions(inputs, DefaultSubstitutions)
	if out["location"] != `"westeurope"` {
		t.Errorf("non-matched attr should be unchanged, got %s", out["location"])
	}
}

func TestApplyDevSubstitutions_OriginalUnmodified(t *testing.T) {
	inputs := map[string]string{
		"account_replication_type": `"GRS"`,
	}
	_ = ApplyDevSubstitutions(inputs, DefaultSubstitutions)
	if inputs["account_replication_type"] != `"GRS"` {
		t.Error("original map should not be modified")
	}
}

func TestApplyDevSubstitutions_CustomRules(t *testing.T) {
	rules := []SubstitutionRule{
		{AttrName: "my_attr", From: "big", To: "small"},
	}
	inputs := map[string]string{"my_attr": `"big"`}
	out := ApplyDevSubstitutions(inputs, rules)
	if out["my_attr"] != `"small"` {
		t.Errorf("custom rule not applied, got %s", out["my_attr"])
	}
}

func TestDowngradeInstanceCount_AboveThreshold(t *testing.T) {
	inputs := map[string]string{"capacity": "5"}
	out := DowngradeInstanceCount(inputs)
	if out["capacity"] != "1" {
		t.Errorf("capacity > 2 should become 1, got %s", out["capacity"])
	}
}

func TestDowngradeInstanceCount_BelowThreshold(t *testing.T) {
	inputs := map[string]string{"capacity": "2"}
	out := DowngradeInstanceCount(inputs)
	if out["capacity"] != "2" {
		t.Errorf("capacity ≤ 2 should be unchanged, got %s", out["capacity"])
	}
}

func TestDowngradeInstanceCount_NonNumericUnchanged(t *testing.T) {
	inputs := map[string]string{"capacity": `"auto"`}
	out := DowngradeInstanceCount(inputs)
	if out["capacity"] != `"auto"` {
		t.Errorf("non-numeric capacity should be unchanged, got %s", out["capacity"])
	}
}
