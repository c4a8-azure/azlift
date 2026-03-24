package enrich

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/refine"
)

func makeLocalsFile(t *testing.T, content string) *refine.ParsedFile {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "locals.tf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing locals.tf: %v", err)
	}
	pf, err := refine.ParseFile(path)
	if err != nil {
		t.Fatalf("parsing locals.tf: %v", err)
	}
	return pf
}

func TestNormaliseTags_InjectsCommonTagsLocal(t *testing.T) {
	locals := makeLocalsFile(t, `locals { environment = "prod" }`)
	hcl := `resource "azurerm_resource_group" "rg" { name = "rg-prod" }`
	files := writeTF(t, hcl)

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

func TestNormaliseTags_ResourceGetsMerge(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	hcl := `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = {
    team = "platform"
  }
}
`
	files := writeTF(t, hcl)
	count := NormaliseTags(files, locals)
	if count != 1 {
		t.Errorf("want 1 normalised, got %d", count)
	}

	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "merge(local.common_tags") {
		t.Errorf("expected merge(local.common_tags, ...) in resource, got:\n%s", out)
	}
}

func TestNormaliseTags_Idempotent(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	hcl := `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = merge(local.common_tags, { team = "platform" })
}
`
	files := writeTF(t, hcl)
	count := NormaliseTags(files, locals)
	if count != 0 {
		t.Errorf("already-normalised resource should not be modified, got count %d", count)
	}
}

func TestNormaliseTags_BespokeTagsPreserved(t *testing.T) {
	locals := makeLocalsFile(t, `locals {}`)
	hcl := `
resource "azurerm_resource_group" "rg" {
  name = "rg-prod"
  tags = {
    team       = "platform"
    cost-center = "123"
  }
}
`
	files := writeTF(t, hcl)
	NormaliseTags(files, locals)

	out := string(files[0].File.Bytes())
	// team is bespoke → should remain in the bespoke object
	if !strings.Contains(out, "team") {
		t.Error("bespoke tag 'team' should be preserved in the merge call")
	}
}

func TestGenerateCommonTagsOutput(t *testing.T) {
	pf := refine.NewFile("/tmp/outputs.tf")
	GenerateCommonTagsOutput(pf)

	out := string(pf.File.Bytes())
	if !strings.Contains(out, "common_tags") {
		t.Error("expected common_tags output block")
	}
	if !strings.Contains(out, "local.common_tags") {
		t.Error("expected output to reference local.common_tags")
	}
}
