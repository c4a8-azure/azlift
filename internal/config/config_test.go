package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	ctx := &PipelineContext{}
	ctx.Defaults()

	if ctx.Mode != ModeModules {
		t.Errorf("Mode: want %q, got %q", ModeModules, ctx.Mode)
	}
	if ctx.Platform != PlatformGitHub {
		t.Errorf("Platform: want %q, got %q", PlatformGitHub, ctx.Platform)
	}
	if len(ctx.Environments) != 3 {
		t.Errorf("Environments: want 3 defaults, got %d", len(ctx.Environments))
	}
	if ctx.ExportOutputDir != "./raw" {
		t.Errorf("ExportOutputDir: want ./raw, got %q", ctx.ExportOutputDir)
	}
}

func TestLoadNoFile(t *testing.T) {
	// Run from a temp dir so no .azlift.yaml exists.
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer func() { _ = os.Chdir(orig) }()

	ctx, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Mode != ModeModules {
		t.Errorf("expected default Mode, got %q", ctx.Mode)
	}
}

func TestLoadFromFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "azlift.yaml")
	content := `
subscription_id: sub-abc
resource_group: rg-test
mode: terragrunt
platform: ado
`
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if ctx.SubscriptionID != "sub-abc" {
		t.Errorf("SubscriptionID: want sub-abc, got %q", ctx.SubscriptionID)
	}
	if ctx.Mode != ModeTerragrunt {
		t.Errorf("Mode: want terragrunt, got %q", ctx.Mode)
	}
	if ctx.Platform != PlatformADO {
		t.Errorf("Platform: want ado, got %q", ctx.Platform)
	}
}

func TestMergeFlags_OverridesFile(t *testing.T) {
	ctx := &PipelineContext{
		SubscriptionID: "from-file",
		Mode:           ModeModules,
	}
	MergeFlags(ctx, Flags{
		SubscriptionID: "from-flag",
		Mode:           "terragrunt",
	})
	if ctx.SubscriptionID != "from-flag" {
		t.Errorf("SubscriptionID: flag should win, got %q", ctx.SubscriptionID)
	}
	if ctx.Mode != ModeTerragrunt {
		t.Errorf("Mode: flag should win, got %q", ctx.Mode)
	}
}

func TestMergeFlags_EmptyFlagsDoNotOverride(t *testing.T) {
	ctx := &PipelineContext{SubscriptionID: "from-file"}
	MergeFlags(ctx, Flags{}) // all zero
	if ctx.SubscriptionID != "from-file" {
		t.Errorf("SubscriptionID should be preserved, got %q", ctx.SubscriptionID)
	}
}

func TestValidate(t *testing.T) {
	base := func() *PipelineContext {
		return &PipelineContext{
			SubscriptionID: "sub-x",
			ResourceGroup:  "rg-x",
			RepoName:       "repo-x",
			Mode:           ModeModules,
			Platform:       PlatformGitHub,
		}
	}

	if err := Validate(base(), "scan"); err != nil {
		t.Errorf("valid scan context: unexpected error: %v", err)
	}

	noSub := base()
	noSub.SubscriptionID = ""
	if err := Validate(noSub, "scan"); err == nil {
		t.Error("missing subscription: expected error")
	}

	noRG := base()
	noRG.ResourceGroup = ""
	if err := Validate(noRG, "run"); err == nil {
		t.Error("missing resource-group for run: expected error")
	}

	badMode := base()
	badMode.Mode = "invalid"
	if err := Validate(badMode, "scan"); err == nil {
		t.Error("invalid mode: expected error")
	}
}
