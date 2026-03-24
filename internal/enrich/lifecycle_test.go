package enrich

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/refine"
)

func writeTF(t *testing.T, content string) []*refine.ParsedFile {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "main.tf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	files, err := refine.ParseDir(tmp)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	return files
}

func TestInjectPreventDestroy_StatefulGetsLifecycle(t *testing.T) {
	hcl := `
resource "azurerm_storage_account" "sa" {
  name = "stprod"
}
`
	files := writeTF(t, hcl)
	count := InjectPreventDestroy(files, nil)
	if count != 1 {
		t.Errorf("want 1 modified block, got %d", count)
	}
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "lifecycle") {
		t.Error("expected lifecycle block")
	}
	if !strings.Contains(out, "prevent_destroy") {
		t.Error("expected prevent_destroy attribute")
	}
}

func TestInjectPreventDestroy_NonStatefulUnchanged(t *testing.T) {
	hcl := `
resource "azurerm_virtual_network" "vnet" {
  name = "vnet-prod"
}
`
	files := writeTF(t, hcl)
	count := InjectPreventDestroy(files, nil)
	if count != 0 {
		t.Errorf("non-stateful resource should not be modified, got count %d", count)
	}
	out := string(files[0].File.Bytes())
	if strings.Contains(out, "lifecycle") {
		t.Error("non-stateful resource should not get lifecycle block")
	}
}

func TestInjectPreventDestroy_Idempotent(t *testing.T) {
	hcl := `
resource "azurerm_key_vault" "kv" {
  name = "kv-prod"
  lifecycle {
    prevent_destroy = true
  }
}
`
	files := writeTF(t, hcl)
	count := InjectPreventDestroy(files, nil)
	if count != 0 {
		t.Errorf("already-protected resource should not be modified, got count %d", count)
	}
}

func TestInjectPreventDestroy_ExistingLifecycleExtended(t *testing.T) {
	hcl := `
resource "azurerm_cosmosdb_account" "cosmos" {
  name = "cosmos-prod"
  lifecycle {
    ignore_changes = [tags]
  }
}
`
	files := writeTF(t, hcl)
	count := InjectPreventDestroy(files, nil)
	if count != 1 {
		t.Errorf("want 1 modified, got %d", count)
	}
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "prevent_destroy") {
		t.Error("prevent_destroy should be added to existing lifecycle")
	}
	if !strings.Contains(out, "ignore_changes") {
		t.Error("existing ignore_changes should be preserved")
	}
}

func TestInjectPreventDestroy_CustomTypes(t *testing.T) {
	hcl := `resource "my_custom_type" "x" { name = "x" }`
	files := writeTF(t, hcl)
	custom := map[string]bool{"my_custom_type": true}
	count := InjectPreventDestroy(files, custom)
	if count != 1 {
		t.Errorf("custom stateful type should be modified, got count %d", count)
	}
}
