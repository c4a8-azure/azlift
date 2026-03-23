package refine

import (
	"testing"
)

func TestParseName_FullCAFPattern(t *testing.T) {
	p := ParseName("rg-prod-network-westeu-001")
	if p.Environment != "prod" {
		t.Errorf("environment: want prod, got %q", p.Environment)
	}
	if p.Region != "westeurope" {
		t.Errorf("region: want westeurope, got %q", p.Region)
	}
	if p.Prefix != "network" {
		t.Errorf("prefix: want network, got %q", p.Prefix)
	}
}

func TestParseName_VNet(t *testing.T) {
	p := ParseName("vnet-dev-hub-eastus")
	if p.Environment != "dev" {
		t.Errorf("environment: want dev, got %q", p.Environment)
	}
	if p.Region != "eastus" {
		t.Errorf("region: want eastus, got %q", p.Region)
	}
}

func TestParseName_NoRegion(t *testing.T) {
	p := ParseName("kv-prod-secrets")
	if p.Environment != "prod" {
		t.Errorf("environment: want prod, got %q", p.Environment)
	}
	if p.Region != "" {
		t.Errorf("region: want empty, got %q", p.Region)
	}
	if p.Prefix != "secrets" {
		t.Errorf("prefix: want secrets, got %q", p.Prefix)
	}
}

func TestParseName_NoMatch(t *testing.T) {
	p := ParseName("mystorage")
	if p.Environment != "" || p.Region != "" {
		t.Errorf("unexpected parse of non-CAF name: %+v", p)
	}
}

func TestParseName_Empty(t *testing.T) {
	p := ParseName("")
	if p.Environment != "" || p.Region != "" || p.Prefix != "" {
		t.Errorf("expected zero value for empty name, got %+v", p)
	}
}

func TestParseName_NumericSuffixStripped(t *testing.T) {
	p := ParseName("vm-stg-web-uksouth-001")
	if p.Prefix != "web" {
		t.Errorf("prefix: want web, got %q", p.Prefix)
	}
}

func TestAnalyseNames_ExtractsFromFiles(t *testing.T) {
	hcl := `
resource "azurerm_resource_group" "a" {
  name     = "rg-prod-network-westeu-001"
  location = "westeurope"
}
resource "azurerm_virtual_network" "b" {
  name     = "vnet-prod-core-westeu"
  location = "westeurope"
}
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	envs, regions := AnalyseNames(files)
	if len(envs) != 1 || envs[0] != "prod" {
		t.Errorf("environments: want [prod], got %v", envs)
	}
	if len(regions) != 1 || regions[0] != "westeurope" {
		t.Errorf("regions: want [westeurope], got %v", regions)
	}
}

func TestAnalyseNames_DeduplicatesAcrossResources(t *testing.T) {
	hcl := `
resource "azurerm_subnet" "a" { name = "snet-dev-app-eastus" }
resource "azurerm_subnet" "b" { name = "snet-dev-db-eastus" }
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	envs, regions := AnalyseNames(files)
	if len(envs) != 1 {
		t.Errorf("want 1 unique env, got %d: %v", len(envs), envs)
	}
	if len(regions) != 1 {
		t.Errorf("want 1 unique region, got %d: %v", len(regions), regions)
	}
}

func TestAnalyseNames_SkipsInterpolated(t *testing.T) {
	hcl := `resource "azurerm_rg" "x" { name = "${var.env}-rg" }`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	envs, regions := AnalyseNames(files)
	if len(envs) != 0 || len(regions) != 0 {
		t.Errorf("interpolated name should be skipped, got envs=%v regions=%v", envs, regions)
	}
}
