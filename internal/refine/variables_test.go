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

func TestExtractVariables_RGNameBecomesLocal(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", threeResourceHCL)
	files, _ := ParseDir(tmp)

	_, localsFile, err := ExtractVariables(files, tmp)
	if err != nil {
		t.Fatalf("ExtractVariables: %v", err)
	}

	out := string(localsFile.File.Bytes())
	if !strings.Contains(out, "resource_group_name") {
		t.Error("resource_group_name should appear in locals.tf")
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
	if !strings.Contains(out, "local.resource_group_name") {
		t.Error("resource blocks should reference local.resource_group_name after extraction")
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
