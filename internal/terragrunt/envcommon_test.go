package terragrunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEnvcommon_CreatesFiles(t *testing.T) {
	tmp := t.TempDir()
	workloads := []WorkloadInputs{
		{Name: "networking", Inputs: map[string]string{"location": `"westeurope"`}},
		{Name: "compute", Inputs: map[string]string{}},
	}

	if err := GenerateEnvcommon(tmp, workloads); err != nil {
		t.Fatalf("GenerateEnvcommon: %v", err)
	}

	for _, name := range []string{"networking.hcl", "compute.hcl"} {
		path := filepath.Join(tmp, "_envcommon", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestGenerateEnvcommon_ContainsInputs(t *testing.T) {
	tmp := t.TempDir()
	workloads := []WorkloadInputs{
		{
			Name: "networking",
			Inputs: map[string]string{
				"location":            `"westeurope"`,
				"resource_group_name": `"rg-prod-network"`,
			},
		},
	}

	if err := GenerateEnvcommon(tmp, workloads); err != nil {
		t.Fatalf("GenerateEnvcommon: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "_envcommon", "networking.hcl"))
	out := string(content)
	if !strings.Contains(out, "location") {
		t.Error("expected location input")
	}
	if !strings.Contains(out, "westeurope") {
		t.Error("expected westeurope value")
	}
}

func TestGenerateEnvcommon_InputsSorted(t *testing.T) {
	tmp := t.TempDir()
	workloads := []WorkloadInputs{
		{
			Name: "data",
			Inputs: map[string]string{
				"z_last":  `"z"`,
				"a_first": `"a"`,
				"m_mid":   `"m"`,
			},
		},
	}

	if err := GenerateEnvcommon(tmp, workloads); err != nil {
		t.Fatalf("GenerateEnvcommon: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "_envcommon", "data.hcl"))
	out := string(content)

	posA := strings.Index(out, "a_first")
	posM := strings.Index(out, "m_mid")
	posZ := strings.Index(out, "z_last")
	if posA > posM || posM > posZ {
		t.Error("inputs should be sorted alphabetically")
	}
}

func TestDefaultWorkloads_FiltersScaffold(t *testing.T) {
	names := []string{
		"networking.tf", "compute.tf", "main.tf",
		"backend.tf", "versions.tf", "providers.tf",
		"variables.tf", "locals.tf",
	}
	workloads := DefaultWorkloads(names, "../modules")
	for _, w := range workloads {
		switch w.Name {
		case "backend", "versions", "providers", "variables", "locals":
			t.Errorf("scaffold file %s should be filtered out", w.Name)
		}
	}
	if len(workloads) == 0 {
		t.Error("expected at least networking and compute workloads")
	}
}

func TestDefaultWorkloads_SetsModuleSource(t *testing.T) {
	workloads := DefaultWorkloads([]string{"networking.tf"}, "../modules")
	if len(workloads) != 1 {
		t.Fatalf("want 1 workload, got %d", len(workloads))
	}
	if !strings.Contains(workloads[0].ModuleSource, "networking") {
		t.Errorf("unexpected module source: %s", workloads[0].ModuleSource)
	}
}
