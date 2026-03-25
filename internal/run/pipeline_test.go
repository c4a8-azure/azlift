package run

import (
	"testing"
)

func TestApplyDefaults_FillsMode(t *testing.T) {
	opts := applyDefaults(Options{})
	if opts.Mode != "modules" {
		t.Errorf("want mode=modules, got %s", opts.Mode)
	}
}

func TestApplyDefaults_FillsLocation(t *testing.T) {
	opts := applyDefaults(Options{})
	if opts.Location != "westeurope" {
		t.Errorf("want location=westeurope, got %s", opts.Location)
	}
}

func TestApplyDefaults_FillsWorkDir(t *testing.T) {
	opts := applyDefaults(Options{})
	if opts.WorkDir != ".azlift" {
		t.Errorf("want workDir=.azlift, got %s", opts.WorkDir)
	}
}

func TestApplyDefaults_FillsEnvironments(t *testing.T) {
	opts := applyDefaults(Options{})
	if len(opts.Environments) != 3 {
		t.Errorf("want 3 default environments, got %d", len(opts.Environments))
	}
}

func TestApplyDefaults_PreservesExistingValues(t *testing.T) {
	opts := applyDefaults(Options{
		Mode:     "terragrunt",
		Location: "uksouth",
		WorkDir:  "/tmp/mywork",
	})
	if opts.Mode != "terragrunt" {
		t.Errorf("mode should not be overwritten, got %s", opts.Mode)
	}
	if opts.Location != "uksouth" {
		t.Errorf("location should not be overwritten, got %s", opts.Location)
	}
	if opts.WorkDir != "/tmp/mywork" {
		t.Errorf("workDir should not be overwritten, got %s", opts.WorkDir)
	}
}

func TestApplyDefaults_TargetSubscriptionDefaultsToSource(t *testing.T) {
	opts := applyDefaults(Options{SubscriptionID: "sub-123"})
	if opts.TargetSubscription != "sub-123" {
		t.Errorf("want TargetSubscription=sub-123, got %s", opts.TargetSubscription)
	}
}

func TestApplyDefaults_TargetSubscriptionNotOverwritten(t *testing.T) {
	opts := applyDefaults(Options{
		SubscriptionID:     "sub-source",
		TargetSubscription: "sub-target",
	})
	if opts.TargetSubscription != "sub-target" {
		t.Errorf("explicit TargetSubscription should not be overwritten")
	}
}
