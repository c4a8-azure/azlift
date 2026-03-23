package enrich

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/refine"
)

func TestEnrichDescriptions_NilClientPassthrough(t *testing.T) {
	hcl := `
variable "location" {
  type = string
}
`
	files := writeTF(t, hcl)
	content, err := EnrichDescriptions(context.Background(), nil, files[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty passthrough content")
	}
}

func TestEnrichDescriptions_NoVarOrOutputSkipped(t *testing.T) {
	hcl := `resource "azurerm_rg" "x" { name = "rg" }`
	files := writeTF(t, hcl)
	orig := string(files[0].File.Bytes())
	content, err := EnrichDescriptions(context.Background(), nil, files[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != orig {
		t.Error("file without variable/output should be returned unchanged")
	}
}

func TestEnrichDescriptionsAll_NilClientNoChange(t *testing.T) {
	hcl := `variable "location" { type = string }`
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "variables.tf"), []byte(hcl), 0o600); err != nil {
		t.Fatal(err)
	}
	files, _ := refine.ParseDir(tmp)

	count, err := EnrichDescriptionsAll(context.Background(), nil, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil client means no change, count = 0
	if count != 0 {
		t.Errorf("nil client should produce 0 enriched files, got %d", count)
	}
}

func TestReplaceContent_UpdatesAST(t *testing.T) {
	hcl := `variable "location" { type = string }`
	files := writeTF(t, hcl)

	newContent := `variable "location" {
  type        = string
  description = "Azure location for all resources."
}`
	if err := refine.ReplaceContent(files[0], []byte(newContent)); err != nil {
		t.Fatalf("ReplaceContent: %v", err)
	}
	out := string(files[0].File.Bytes())
	if !strings.Contains(out, "description") {
		t.Error("AST should contain description after ReplaceContent")
	}
}
