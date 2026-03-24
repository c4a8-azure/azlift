package workflows_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/workflows"
)

func TestRender_ProducesFilesPerEnvironment(t *testing.T) {
	cfg := workflows.Config{Environments: []string{"prod", "staging", "dev"}}
	files, err := workflows.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 3 envs × 2 types = 6 files
	if len(files) != 6 {
		t.Errorf("want 6 files, got %d: %v", len(files), keys(files))
	}
}

func TestRender_PlanFileContainsEnvironmentName(t *testing.T) {
	cfg := workflows.Config{Environments: []string{"prod"}}
	files, err := workflows.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	plan, ok := files["plan-prod.yml"]
	if !ok {
		t.Fatal("expected plan-prod.yml in output")
	}
	if !strings.Contains(string(plan), "prod-iac-plan") {
		t.Errorf("plan-prod.yml should reference prod-iac-plan environment, got:\n%s", plan)
	}
}

func TestRender_ApplyFileContainsEnvironmentName(t *testing.T) {
	cfg := workflows.Config{Environments: []string{"staging"}}
	files, err := workflows.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	apply, ok := files["apply-staging.yml"]
	if !ok {
		t.Fatal("expected apply-staging.yml in output")
	}
	if !strings.Contains(string(apply), "staging-iac-apply") {
		t.Errorf("apply-staging.yml should reference staging-iac-apply environment, got:\n%s", apply)
	}
}

func TestRender_PlanTriggersOnPR(t *testing.T) {
	cfg := workflows.Config{Environments: []string{"prod"}}
	files, _ := workflows.Render(cfg)
	plan := string(files["plan-prod.yml"])
	if !strings.Contains(plan, "pull_request") {
		t.Error("plan workflow should trigger on pull_request")
	}
}

func TestRender_ApplyTriggersOnPush(t *testing.T) {
	cfg := workflows.Config{Environments: []string{"prod"}}
	files, _ := workflows.Render(cfg)
	apply := string(files["apply-prod.yml"])
	if !strings.Contains(apply, "push") {
		t.Error("apply workflow should trigger on push")
	}
}

func TestWrite_CreatesWorkflowsDir(t *testing.T) {
	dir := t.TempDir()
	cfg := workflows.Config{Environments: []string{"prod", "dev"}}

	if err := workflows.Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	wfDir := filepath.Join(dir, ".github", "workflows")
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		t.Fatalf("reading workflows dir: %v", err)
	}
	if len(entries) != 4 { // 2 envs × 2 types
		t.Errorf("want 4 files, got %d", len(entries))
	}
}

func TestRender_CustomDir(t *testing.T) {
	customDir := t.TempDir()
	// Write minimal custom templates.
	planContent := "name: custom plan {{.Environment}}\non: [push]"
	applyContent := "name: custom apply {{.Environment}}\non: [push]"
	if err := os.WriteFile(filepath.Join(customDir, "plan.yml.tmpl"), []byte(planContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "apply.yml.tmpl"), []byte(applyContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := workflows.Config{Environments: []string{"prod"}, CustomDir: customDir}
	files, err := workflows.Render(cfg)
	if err != nil {
		t.Fatalf("Render with custom dir: %v", err)
	}
	if !strings.Contains(string(files["plan-prod.yml"]), "custom plan prod") {
		t.Error("custom template should be used")
	}
}

func keys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
