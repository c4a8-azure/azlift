package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsUnsupported_KnownType(t *testing.T) {
	u, ok := IsUnsupported("microsoft.aad/domainservices")
	if !ok {
		t.Fatal("expected microsoft.aad/domainservices to be unsupported")
	}
	if u.DataSourceType != "azurerm_active_directory_domain_service" {
		t.Errorf("unexpected data source type: %s", u.DataSourceType)
	}
}

func TestIsUnsupported_CaseInsensitive(t *testing.T) {
	_, ok := IsUnsupported("Microsoft.AAD/domainServices")
	if !ok {
		t.Error("IsUnsupported should be case-insensitive")
	}
}

func TestIsUnsupported_UnknownType(t *testing.T) {
	_, ok := IsUnsupported("microsoft.compute/virtualmachines")
	if ok {
		t.Error("VMs should not be in the unsupported registry")
	}
}

func TestGenerateDataStub_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	ref := ResourceRef{
		ID:            "/subscriptions/sub/resourceGroups/rg-a/providers/microsoft.aad/domainservices/my-domain",
		Name:          "my-domain",
		Type:          "microsoft.aad/domainservices",
		ResourceGroup: "rg-a",
	}
	path, err := GenerateDataStub(ref, tmp)
	if err != nil {
		t.Fatalf("GenerateDataStub error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading stub file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "data \"azurerm_active_directory_domain_service\"") {
		t.Error("stub should contain the data source block")
	}
	if !strings.Contains(content, "# TODO(azlift):") {
		t.Error("stub should contain a TODO comment")
	}
	if !strings.Contains(content, "my-domain") {
		t.Error("stub should reference the resource name")
	}
}

func TestGenerateDataStub_UnknownTypeErrors(t *testing.T) {
	_, err := GenerateDataStub(ResourceRef{Type: "microsoft.compute/virtualmachines"}, t.TempDir())
	if err == nil {
		t.Error("expected error for unknown resource type")
	}
}

func TestGenerateDataStub_FilenameSanitised(t *testing.T) {
	tmp := t.TempDir()
	ref := ResourceRef{
		Name:          "My Resource-Name 123",
		Type:          "microsoft.aad/domainservices",
		ResourceGroup: "rg",
	}
	path, err := GenerateDataStub(ref, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Filename must be a valid path component (no spaces or uppercase).
	base := filepath.Base(path)
	if strings.Contains(base, " ") || base != strings.ToLower(base) {
		t.Errorf("filename not sanitised: %s", base)
	}
}

func TestSanitiseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"my-resource", "my_resource"},
		{"My Resource Name", "my_resource_name"},
		{"rg-prod-001", "rg_prod_001"},
		{"___leading", "leading"},
	}
	for _, c := range cases {
		got := sanitiseName(c.in)
		if got != c.want {
			t.Errorf("sanitiseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
