package export

import (
	"testing"
)

func TestExclusionList_DefaultsExcludeDiagnostics(t *testing.T) {
	el := NewExclusionList(nil)
	if !el.IsExcluded("microsoft.insights/diagnosticsettings") {
		t.Error("diagnostic settings should be excluded by default")
	}
	if !el.IsExcluded("Microsoft.Insights/DiagnosticSettings") { // case-insensitive
		t.Error("exclusion should be case-insensitive")
	}
}

func TestExclusionList_DefaultsExcludeRoleAssignments(t *testing.T) {
	el := NewExclusionList(nil)
	if !el.IsExcluded("microsoft.authorization/roleassignments") {
		t.Error("role assignments should be excluded by default")
	}
}

func TestExclusionList_AllowsNonExcludedTypes(t *testing.T) {
	el := NewExclusionList(nil)
	if el.IsExcluded("microsoft.compute/virtualmachines") {
		t.Error("VMs should not be excluded")
	}
	if el.IsExcluded("microsoft.network/virtualnetworks") {
		t.Error("VNets should not be excluded")
	}
}

func TestExclusionList_CustomTypesAdded(t *testing.T) {
	el := NewExclusionList([]string{"microsoft.customproviders/resourceproviders"})
	if !el.IsExcluded("microsoft.customproviders/resourceproviders") {
		t.Error("custom type should be excluded")
	}
}

func TestExclusionList_CustomTypesCaseInsensitive(t *testing.T) {
	el := NewExclusionList([]string{"Microsoft.Foo/Bar"})
	if !el.IsExcluded("microsoft.foo/bar") {
		t.Error("custom exclusion should be case-insensitive")
	}
}

func TestDefaultExcludedTypes_AllNormalised(t *testing.T) {
	for _, typ := range DefaultExcludedTypes {
		if typ != toLower(typ) {
			t.Errorf("DefaultExcludedType %q is not lowercase — normalise it", typ)
		}
	}
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
