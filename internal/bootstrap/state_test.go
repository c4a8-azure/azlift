package bootstrap

import (
	"context"
	"testing"
)

func TestDeriveStateConfig_BasicRepo(t *testing.T) {
	cfg := DeriveStateConfig("sub-123", "infra-prod", "")
	if cfg.Location != "westeurope" {
		t.Errorf("default location: want westeurope, got %s", cfg.Location)
	}
	if cfg.ContainerName != "tfstate" {
		t.Errorf("container: want tfstate, got %s", cfg.ContainerName)
	}
	if cfg.ResourceGroupName != "rg-tfstate-infra-prod" {
		t.Errorf("rg: want rg-tfstate-infra-prod, got %s", cfg.ResourceGroupName)
	}
	// SA: "st" + "infraprod"
	if cfg.StorageAccountName != "stinfraprod" {
		t.Errorf("sa: want stinfraprod, got %s", cfg.StorageAccountName)
	}
}

func TestDeriveStateConfig_LongRepoName(t *testing.T) {
	cfg := DeriveStateConfig("sub-123", "this-is-a-very-long-repository-name-for-infrastructure", "eastus")
	if err := ValidateStateConfig(cfg); err != nil {
		t.Errorf("long repo name produced invalid config: %v", err)
	}
	if len(cfg.StorageAccountName) > 24 {
		t.Errorf("storage account name too long: %s (%d chars)", cfg.StorageAccountName, len(cfg.StorageAccountName))
	}
}

func TestDeriveStateConfig_SpecialChars(t *testing.T) {
	cfg := DeriveStateConfig("sub-123", "My_Infra.Repo!", "")
	if cfg.ResourceGroupName != "rg-tfstate-my-infra-repo" {
		t.Errorf("unexpected rg: %s", cfg.ResourceGroupName)
	}
}

func TestDeriveStateConfig_CustomLocation(t *testing.T) {
	cfg := DeriveStateConfig("sub-123", "myrepo", "uksouth")
	if cfg.Location != "uksouth" {
		t.Errorf("want uksouth, got %s", cfg.Location)
	}
}

func TestValidateStateConfig_Valid(t *testing.T) {
	cfg := DeriveStateConfig("sub-123", "infra-prod", "")
	if err := ValidateStateConfig(cfg); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}
}

func TestValidateStateConfig_MissingSubscription(t *testing.T) {
	cfg := DeriveStateConfig("", "infra-prod", "")
	if err := ValidateStateConfig(cfg); err == nil {
		t.Error("missing subscription should return error")
	}
}

func TestValidateStateConfig_InvalidSAName(t *testing.T) {
	cfg := StateStorageConfig{
		SubscriptionID:     "sub-123",
		ResourceGroupName:  "rg-tfstate",
		StorageAccountName: "st-invalid-name", // hyphens not allowed
	}
	if err := ValidateStateConfig(cfg); err == nil {
		t.Error("invalid SA name should return error")
	}
}

func TestProvisionStateStorage_CallsRunner(t *testing.T) {
	m := &MockRunner{}
	cfg := DeriveStateConfig("sub-123", "infra-prod", "")

	if err := ProvisionStateStorage(context.Background(), m, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(m.Calls))
	}
	if m.Calls[0][0] != "state-storage" {
		t.Errorf("first arg should be state-storage, got %s", m.Calls[0][0])
	}
}

func TestProvisionStateStorage_RunnerErrorPropagated(t *testing.T) {
	m := &MockRunner{Err: &ExitError{Code: 1, Stderr: "subscription not found"}}
	cfg := DeriveStateConfig("sub-123", "infra-prod", "")
	err := ProvisionStateStorage(context.Background(), m, cfg, nil)
	if err == nil {
		t.Error("expected error from runner")
	}
}
