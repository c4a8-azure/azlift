package terragrunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testWorkloads = []WorkloadInputs{
	{Name: "networking", Inputs: map[string]string{}},
	{Name: "compute", Inputs: map[string]string{}},
}

func TestGenerateEnvironments_CreatesEnvDirs(t *testing.T) {
	tmp := t.TempDir()
	envs := DefaultEnvironments()

	if err := GenerateEnvironments(tmp, envs, testWorkloads); err != nil {
		t.Fatalf("GenerateEnvironments: %v", err)
	}

	for _, env := range []string{"prod", "staging", "dev"} {
		if _, err := os.Stat(filepath.Join(tmp, env)); err != nil {
			t.Errorf("missing env dir %s", env)
		}
	}
}

func TestGenerateEnvironments_CreatesEnvHCL(t *testing.T) {
	tmp := t.TempDir()
	envs := []Environment{
		{Name: "prod", SubscriptionID: "sub-123", Tags: map[string]string{"env": "prod"}},
	}

	if err := GenerateEnvironments(tmp, envs, testWorkloads); err != nil {
		t.Fatalf("GenerateEnvironments: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "prod", "env.hcl"))
	out := string(content)
	for _, want := range []string{"prod", "sub-123", `env = "prod"`} {
		if !strings.Contains(out, want) {
			t.Errorf("env.hcl missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestGenerateEnvironments_PlaceholderSubscription(t *testing.T) {
	tmp := t.TempDir()
	envs := []Environment{{Name: "dev"}}

	if err := GenerateEnvironments(tmp, envs, testWorkloads); err != nil {
		t.Fatalf("GenerateEnvironments: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "dev", "env.hcl"))
	if !strings.Contains(string(content), "<subscription_id>") {
		t.Error("empty SubscriptionID should produce placeholder")
	}
}

func TestGenerateEnvironments_CreatesWorkloadTG(t *testing.T) {
	tmp := t.TempDir()
	envs := []Environment{{Name: "prod"}}

	if err := GenerateEnvironments(tmp, envs, testWorkloads); err != nil {
		t.Fatalf("GenerateEnvironments: %v", err)
	}

	for _, w := range []string{"networking", "compute"} {
		tg := filepath.Join(tmp, "prod", w, "terragrunt.hcl")
		if _, err := os.Stat(tg); err != nil {
			t.Errorf("missing %s: %v", tg, err)
		}
	}
}

func TestGenerateEnvironments_WorkloadTGReferencesEnvcommon(t *testing.T) {
	tmp := t.TempDir()
	envs := []Environment{{Name: "staging"}}
	workloads := []WorkloadInputs{{Name: "networking", Inputs: map[string]string{}}}

	if err := GenerateEnvironments(tmp, envs, workloads); err != nil {
		t.Fatalf("GenerateEnvironments: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "staging", "networking", "terragrunt.hcl"))
	out := string(content)
	if !strings.Contains(out, "_envcommon/networking.hcl") {
		t.Error("workload terragrunt.hcl should reference _envcommon/networking.hcl")
	}
	if !strings.Contains(out, "env.hcl") {
		t.Error("workload terragrunt.hcl should reference env.hcl")
	}
}

func TestDefaultEnvironments_ThreeTiers(t *testing.T) {
	envs := DefaultEnvironments()
	if len(envs) != 3 {
		t.Fatalf("want 3 environments, got %d", len(envs))
	}
	names := map[string]bool{}
	for _, e := range envs {
		names[e.Name] = true
	}
	for _, want := range []string{"prod", "staging", "dev"} {
		if !names[want] {
			t.Errorf("missing environment %s", want)
		}
	}
}
