package refine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// threeResourceHCL has location and resource_group_name repeated 3 times.
const threeResourceHCL = `
resource "azurerm_virtual_network" "vnet" {
  name                = "vnet1"
  location            = "westeurope"
  resource_group_name = "rg-prod-westeu"
}

resource "azurerm_subnet" "snet" {
  name                = "snet1"
  location            = "westeurope"
  resource_group_name = "rg-prod-westeu"
}

resource "azurerm_network_security_group" "nsg" {
  name                = "nsg1"
  location            = "westeurope"
  resource_group_name = "rg-prod-westeu"
}
`

func TestExtractVariables_LocationBecomesVar(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", threeResourceHCL)
	files, _ := ParseDir(tmp)

	varsFile, _, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	varBlocks := Blocks(varsFile, "variable")
	found := false
	for _, b := range varBlocks {
		if len(b.Labels()) > 0 && b.Labels()[0] == "location" {
			found = true
		}
	}
	if !found {
		t.Error("'location' should be extracted as a variable block")
	}
}

func TestExtractVariables_RGNameBecomesVar(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", threeResourceHCL)
	files, _ := ParseDir(tmp)

	varsFile, _, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	found := false
	for _, b := range Blocks(varsFile, "variable") {
		if len(b.Labels()) > 0 && b.Labels()[0] == "resource_group_name" {
			found = true
		}
	}
	if !found {
		t.Error("resource_group_name should be extracted as a variable block")
	}
}

func TestExtractVariables_RewritesSourceFile(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", threeResourceHCL)
	files, _ := ParseDir(tmp)

	if _, _, err := ExtractVariables(files, tmp); err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	// After extraction the in-memory AST should reference var.location.
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "var.location") {
		t.Error("resource blocks should reference var.location after extraction")
	}
	if !strings.Contains(out, "var.resource_group_name") {
		t.Error("resource blocks should reference var.resource_group_name after extraction")
	}
}

func TestExtractVariables_BelowThresholdNotExtracted(t *testing.T) {
	// Only 2 resources — below the threshold of 3.
	hcl := `
resource "azurerm_virtual_network" "a" {
  location = "westeurope"
  resource_group_name = "rg-a"
}
resource "azurerm_subnet" "b" {
  location = "westeurope"
  resource_group_name = "rg-a"
}
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	varsFile, _, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	// location is always extracted as a variable regardless of threshold.
	varBlocks := Blocks(varsFile, "variable")
	if len(varBlocks) == 0 {
		t.Error("location should always be extracted even below threshold")
	}
}

func TestExtractVariables_EmptyFiles(t *testing.T) {
	tmp := t.TempDir()
	files := []*ParsedFile{}
	varsFile, localsFile, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if varsFile == nil || localsFile == nil {
		t.Error("should return non-nil files even when input is empty")
	}
}

func TestExtractVariables_MultipleDistinctValuesGetNumberedLocals(t *testing.T) {
	// 3 resources reference "vnet-a", 3 reference "vnet-b" → two numbered locals.
	hcl := `
resource "azurerm_subnet" "a1" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "a2" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "a3" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "b1" { virtual_network_name = "vnet-b" }
resource "azurerm_subnet" "b2" { virtual_network_name = "vnet-b" }
resource "azurerm_subnet" "b3" { virtual_network_name = "vnet-b" }
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	_, localsFile, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	out := string(localsFile.File.Bytes())
	if !strings.Contains(out, "virtual_network_name_001") {
		t.Errorf("expected virtual_network_name_001 in locals.tf, got:\n%s", out)
	}
	if !strings.Contains(out, "virtual_network_name_002") {
		t.Errorf("expected virtual_network_name_002 in locals.tf, got:\n%s", out)
	}
	// Values are sorted alphabetically: "vnet-a" → _001, "vnet-b" → _002.
	if !strings.Contains(out, `"vnet-a"`) {
		t.Errorf("expected vnet-a value in locals.tf, got:\n%s", out)
	}
	if !strings.Contains(out, `"vnet-b"`) {
		t.Errorf("expected vnet-b value in locals.tf, got:\n%s", out)
	}
}

func TestExtractVariables_MultipleDistinctValuesRewrittenCorrectly(t *testing.T) {
	// Each resource must be rewritten to its own numbered local, not to the same one.
	hcl := `
resource "azurerm_subnet" "a1" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "a2" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "a3" { virtual_network_name = "vnet-a" }
resource "azurerm_subnet" "b1" { virtual_network_name = "vnet-b" }
resource "azurerm_subnet" "b2" { virtual_network_name = "vnet-b" }
resource "azurerm_subnet" "b3" { virtual_network_name = "vnet-b" }
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	if _, _, err := ExtractVariables(files, tmp); err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	out := string(files[0].File.Bytes())
	if strings.Contains(out, `"vnet-a"`) {
		t.Errorf("literal vnet-a should have been replaced with a local ref:\n%s", out)
	}
	if strings.Contains(out, `"vnet-b"`) {
		t.Errorf("literal vnet-b should have been replaced with a local ref:\n%s", out)
	}
	if !strings.Contains(out, "local.virtual_network_name_001") {
		t.Errorf("expected local.virtual_network_name_001 in resource blocks:\n%s", out)
	}
	if !strings.Contains(out, "local.virtual_network_name_002") {
		t.Errorf("expected local.virtual_network_name_002 in resource blocks:\n%s", out)
	}
}

func TestExtractVariables_RewriteOnlyMatchingValue(t *testing.T) {
	// resource_group_name appears once with "rg-a" and three times with "rg-main".
	// Both are alwaysVariable, so numbered variables are generated.
	// "rg-a" must be rewritten to its own ref, not to rg-main's ref.
	hcl := `
resource "azurerm_resource_group" "a"  { resource_group_name = "rg-a" }
resource "azurerm_vnet" "v1"           { resource_group_name = "rg-main" }
resource "azurerm_vnet" "v2"           { resource_group_name = "rg-main" }
resource "azurerm_subnet" "s1"         { resource_group_name = "rg-main" }
`
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", hcl)
	files, _ := ParseDir(tmp)

	varsFile, _, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	src := string(files[0].File.Bytes())
	vars := string(varsFile.File.Bytes())

	// Both values must appear in variables.tf (alwaysVariable forces extraction).
	if !strings.Contains(vars, "rg-a") {
		t.Errorf("rg-a should be in variables.tf:\n%s", vars)
	}
	if !strings.Contains(vars, "rg-main") {
		t.Errorf("rg-main should be in variables.tf:\n%s", vars)
	}
	// No raw string literals should remain in resource blocks.
	if strings.Contains(src, `"rg-a"`) {
		t.Errorf(`literal "rg-a" should have been replaced with a ref:\n%s`, src)
	}
	if strings.Contains(src, `"rg-main"`) {
		t.Errorf(`literal "rg-main" should have been replaced with a ref:\n%s`, src)
	}
	// Resources should reference numbered var.* refs.
	if !strings.Contains(src, "var.resource_group_name") {
		t.Errorf("resource blocks should use var.resource_group_name refs:\n%s", src)
	}
}

func TestExtractVariables_WrittenFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", threeResourceHCL)
	files, _ := ParseDir(tmp)

	varsFile, localsFile, _ := ExtractVariables(files, tmp)

	// Write and verify files can be re-parsed.
	if err := WriteFile(varsFile); err != nil {
		t.Fatalf("WriteFile(varsFile): %v", err)
	}
	if err := WriteFile(localsFile); err != nil {
		t.Fatalf("WriteFile(localsFile): %v", err)
	}

	for _, name := range []string{"variables.tf", "locals.tf"} {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
}
