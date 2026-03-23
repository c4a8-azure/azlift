package refine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleHCL = `
resource "azurerm_resource_group" "example" {
  name     = "rg-prod-westeu"
  location = "westeurope"
}

resource "azurerm_virtual_network" "example" {
  name                = "vnet-prod-westeu"
  location            = "westeurope"
  resource_group_name = "rg-prod-westeu"
  address_space       = ["10.0.0.0/16"]
}
`

func writeTFFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	return path
}

func TestParseFile_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTFFile(t, tmp, "main.tf", sampleHCL)

	pf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Path != path {
		t.Errorf("path: want %s, got %s", path, pf.Path)
	}
	blocks := Blocks(pf, "resource")
	if len(blocks) != 2 {
		t.Errorf("want 2 resource blocks, got %d", len(blocks))
	}
}

func TestParseFile_Invalid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTFFile(t, tmp, "bad.tf", `resource "azurerm_rg" {  # missing closing brace`)
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected parse error for invalid HCL")
	}
}

func TestParseDir_MultipleFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", sampleHCL)
	writeTFFile(t, tmp, "variables.tf", `variable "location" { type = string }`)

	files, err := ParseDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("want 2 files, got %d", len(files))
	}
}

func TestParseDir_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	files, err := ParseDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error on empty dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("want 0 files, got %d", len(files))
	}
}

func TestRoundTrip_PreservesContent(t *testing.T) {
	tmp := t.TempDir()
	path := writeTFFile(t, tmp, "main.tf", sampleHCL)

	pf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dest := filepath.Join(tmp, "out.tf")
	if err := WriteFileTo(pf, dest); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	// Round-trip should preserve all resource types and names.
	if !strings.Contains(string(out), "azurerm_resource_group") {
		t.Error("output missing azurerm_resource_group")
	}
	if !strings.Contains(string(out), "azurerm_virtual_network") {
		t.Error("output missing azurerm_virtual_network")
	}
}

func TestWriteDir_CreatesFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeTFFile(t, src, "main.tf", sampleHCL)
	writeTFFile(t, src, "variables.tf", `variable "env" { type = string }`)

	files, _ := ParseDir(src)
	if err := WriteDir(files, dst); err != nil {
		t.Fatalf("WriteDir: %v", err)
	}

	for _, name := range []string{"main.tf", "variables.tf"} {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("missing output file %s", name)
		}
	}
}

func TestAllBlocks_AcrossFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "a.tf", `resource "azurerm_rg" "a" { name = "a" }`)
	writeTFFile(t, tmp, "b.tf", `resource "azurerm_rg" "b" { name = "b" }`)

	files, _ := ParseDir(tmp)
	blocks := AllBlocks(files, "resource")
	if len(blocks) != 2 {
		t.Errorf("want 2 resource blocks across files, got %d", len(blocks))
	}
}

func TestNewFile_Empty(t *testing.T) {
	pf := NewFile("/tmp/test.tf")
	if pf.File == nil {
		t.Error("expected non-nil hclwrite.File")
	}
	if len(pf.File.Body().Blocks()) != 0 {
		t.Error("new file should have no blocks")
	}
}
