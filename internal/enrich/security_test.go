package enrich

import (
	"strings"
	"testing"
)

const secHCL = `
resource "azurerm_storage_account" "sa" {
  name                     = "stprod"
  allow_blob_public_access = true
  enable_https_traffic_only = false
  min_tls_version          = "TLS1_0"
}
`

func TestScanSecurity_DetectsAntiPatterns(t *testing.T) {
	files := writeTF(t, secHCL)
	findings := ScanSecurity(files, nil, false)
	if len(findings) == 0 {
		t.Fatal("expected security findings, got none")
	}
}

func TestScanSecurity_FindsBlobPublicAccess(t *testing.T) {
	files := writeTF(t, secHCL)
	findings := ScanSecurity(files, nil, false)

	found := false
	for _, f := range findings {
		if f.Attribute == "allow_blob_public_access" {
			found = true
		}
	}
	if !found {
		t.Error("expected finding for allow_blob_public_access")
	}
}

func TestScanSecurity_AutoFixApplied(t *testing.T) {
	files := writeTF(t, secHCL)
	findings := ScanSecurity(files, nil, true)

	fixed := 0
	for _, f := range findings {
		if f.Fixed {
			fixed++
		}
	}
	if fixed == 0 {
		t.Error("expected at least one auto-fix to be applied")
	}

	// Verify the fix is reflected in the AST.
	out := string(files[0].File.Bytes())
	if strings.Contains(out, "allow_blob_public_access = true") {
		t.Error("allow_blob_public_access should have been fixed to false")
	}
}

func TestScanSecurity_CleanFileNoFindings(t *testing.T) {
	hcl := `
resource "azurerm_storage_account" "sa" {
  name                      = "stprod"
  allow_blob_public_access  = false
  enable_https_traffic_only = true
  min_tls_version           = "TLS1_2"
}
`
	files := writeTF(t, hcl)
	findings := ScanSecurity(files, nil, false)
	if len(findings) != 0 {
		t.Errorf("expected no findings for clean file, got %d", len(findings))
	}
}

func TestScanSecurity_NonResourceBlockSkipped(t *testing.T) {
	hcl := `
variable "allow_blob_public_access" {
  default = true
}
`
	files := writeTF(t, hcl)
	findings := ScanSecurity(files, nil, false)
	if len(findings) != 0 {
		t.Errorf("variable block should not trigger security findings, got %d", len(findings))
	}
}

func TestFormatFindings_Empty(t *testing.T) {
	out := FormatFindings(nil)
	if !strings.Contains(out, "no issues") {
		t.Errorf("expected 'no issues', got %q", out)
	}
}

func TestFormatFindings_WithFindings(t *testing.T) {
	findings := []SecurityFinding{
		{
			ResourceType: "azurerm_storage_account",
			ResourceName: "sa",
			Attribute:    "allow_blob_public_access",
			Message:      "Blob public access is enabled",
		},
	}
	out := FormatFindings(findings)
	if !strings.Contains(out, "1 issue") {
		t.Errorf("expected issue count in output, got %q", out)
	}
	if !strings.Contains(out, "azurerm_storage_account") {
		t.Errorf("expected resource type in output, got %q", out)
	}
}
