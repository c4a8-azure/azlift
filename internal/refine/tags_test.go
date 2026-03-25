package refine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeLocalsFile(t *testing.T, content string) *ParsedFile {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "locals.tf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing locals.tf: %v", err)
	}
	pf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("parsing locals.tf: %v", err)
	}
	return pf
}

func parseHCL(t *testing.T, hcl string) []*ParsedFile {
	t.Helper()
	tmp := t.TempDir()
	writeTFFile(t, tmp, "res.tf", hcl)
	files, err := ParseDir(tmp)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	return files
}

func TestNormaliseTags_InjectsCommonTagsLocal(t *testing.T) {
	locals := makeLocalsFile(t, `locals { environment = "prod" }`)
	files := parseHCL(t, `resource "azurerm_resource_group" "rg" { name = "rg-prod" }`)

	NormaliseTags(files, locals)

	out := string(locals.File.Bytes())
	if !strings.Contains(out, "common_tags") {
		t.Error("expected common_tags in locals.tf")
	}
	for _, key := range StandardTagKeys {
		if !strings.Contains(out, key) {
			t.Errorf("expected standard tag key %q in common_tags", key)
		}
	}
}

func TestNormaliseTags_ResourceWithTagsGetsMerge(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	files := parseHCL(t, `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = {
    team = "platform"
  }
}
`)
	count := NormaliseTags(files, locals)
	if count != 1 {
		t.Errorf("want 1 normalised, got %d", count)
	}
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "merge(local.common_tags") {
		t.Errorf("expected merge(local.common_tags, ...) in resource, got:\n%s", out)
	}
	if !strings.Contains(out, "team") {
		t.Error("bespoke tag 'team' should be preserved inside the merge call")
	}
}

func TestNormaliseTags_ResourceWithoutTagsGetsMerge(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	files := parseHCL(t, `resource "azurerm_resource_group" "rg" { name = "rg-prod" }`)
	count := NormaliseTags(files, locals)
	if count != 1 {
		t.Errorf("want 1 normalised, got %d", count)
	}
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "merge(local.common_tags, {})") {
		t.Errorf("expected merge(local.common_tags, {}) for resource with no tags, got:\n%s", out)
	}
}

func TestNormaliseTags_Idempotent(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	files := parseHCL(t, `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = merge(local.common_tags, { team = "platform" })
}
`)
	count := NormaliseTags(files, locals)
	if count != 0 {
		t.Errorf("already-normalised resource should not be modified, got count %d", count)
	}
}

func TestNormaliseTags_SkipsNoTagsResources(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	files := parseHCL(t, `
resource "azurerm_subnet" "snet" {
  name                 = "snet-prod"
  resource_group_name  = "rg-prod"
  virtual_network_name = "vnet-prod"
  address_prefixes     = ["10.0.1.0/24"]
}
resource "azurerm_storage_container" "container" {
  name                 = "tfstate"
  storage_account_name = "stprod"
}
resource "azurerm_virtual_network_peering" "peer" {
  name                      = "peer-to-hub"
  resource_group_name       = "rg-prod"
  virtual_network_name      = "vnet-prod"
  remote_virtual_network_id = "/subscriptions/x/resourceGroups/rg-hub/providers/Microsoft.Network/virtualNetworks/vnet-hub"
}
resource "azurerm_storage_account_queue_properties" "props" {
  storage_account_id = "/subscriptions/x/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/st"
}
`)
	count := NormaliseTags(files, locals)
	if count != 0 {
		t.Errorf("want 0 normalised (all resources are no-tags types), got %d", count)
	}
	out := string(files[0].File.Bytes())
	if strings.Contains(out, "merge(local.common_tags") {
		t.Errorf("no-tags resource types must not get a tags attribute injected:\n%s", out)
	}
}

func TestNormaliseTags_StandardKeysStripped(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	files := parseHCL(t, `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = {
    team        = "platform"
    cost-center = "123"
  }
}
`)
	NormaliseTags(files, locals)

	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "team") {
		t.Error("bespoke tag 'team' should be preserved in the merge call")
	}
}
