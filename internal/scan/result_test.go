package scan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testGroups() map[string]*ResourceGroup {
	return map[string]*ResourceGroup{
		"rg-network": {Name: "rg-network", ResourceCount: 2, ResourceTypes: []string{"microsoft.network/virtualnetworks"}},
		"rg-app":     {Name: "rg-app", ResourceCount: 5, ResourceTypes: []string{"microsoft.compute/virtualmachines"}},
	}
}

func testGraph() *DependencyGraph {
	return &DependencyGraph{Edges: []Dependency{
		{From: "rg-app", To: "rg-network", Reason: "VNet/Subnet reference"},
	}}
}

func TestBuildResult_Fields(t *testing.T) {
	groups := testGroups()
	graph := testGraph()
	result := BuildResult("sub-123", groups, graph)

	if result.SchemaVersion != ResultSchemaVersion {
		t.Errorf("SchemaVersion: want %s, got %s", ResultSchemaVersion, result.SchemaVersion)
	}
	if result.SubscriptionID != "sub-123" {
		t.Errorf("SubscriptionID: want sub-123, got %s", result.SubscriptionID)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	if len(result.ResourceGroups) != 2 {
		t.Errorf("want 2 RGs, got %d", len(result.ResourceGroups))
	}
	if len(result.Boundaries) != 2 {
		t.Errorf("want 2 boundaries, got %d", len(result.Boundaries))
	}
}

func TestBuildResult_BoundaryDependencies(t *testing.T) {
	result := BuildResult("sub-x", testGroups(), testGraph())

	var appBoundary *StateBoundary
	for i := range result.Boundaries {
		if result.Boundaries[i].Name == "rg-app" {
			appBoundary = &result.Boundaries[i]
		}
	}
	if appBoundary == nil {
		t.Fatal("rg-app boundary not found")
	}
	if len(appBoundary.DependsOn) != 1 || appBoundary.DependsOn[0] != "rg-network" {
		t.Errorf("rg-app should depend on rg-network, got: %v", appBoundary.DependsOn)
	}
}

func TestSaveResult_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	result := BuildResult("sub-123", testGroups(), testGraph())
	path, err := SaveResult(result, tmp)
	if err != nil {
		t.Fatalf("SaveResult error: %v", err)
	}
	if !strings.HasSuffix(path, ResultFileName) {
		t.Errorf("unexpected path: %s", path)
	}
	data, err := os.ReadFile(filepath.Join(tmp, ResultFileName))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	var roundtrip ScanResult
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("saved JSON is not valid: %v", err)
	}
	if roundtrip.SchemaVersion != ResultSchemaVersion {
		t.Errorf("round-tripped schemaVersion mismatch")
	}
}

func TestSaveResult_IsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	result := BuildResult("sub-123", testGroups(), testGraph())
	if _, err := SaveResult(result, tmp); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := SaveResult(result, tmp); err != nil {
		t.Fatalf("second save (idempotent): %v", err)
	}
}

func TestPrintRecommendations_Output(t *testing.T) {
	result := BuildResult("sub-123", testGroups(), testGraph())
	var buf bytes.Buffer
	PrintRecommendations(&buf, result)
	out := buf.String()
	if !strings.Contains(out, "rg-app") {
		t.Error("output should mention rg-app")
	}
	if !strings.Contains(out, "rg-network") {
		t.Error("output should mention rg-network")
	}
	if !strings.Contains(out, "VNet/Subnet reference") {
		t.Error("output should show dependency reason")
	}
}

func TestScanResult_JSONRoundtrip(t *testing.T) {
	result := &ScanResult{
		SchemaVersion:  "1",
		ScannedAt:      time.Now().UTC().Truncate(time.Second),
		SubscriptionID: "sub-abc",
		Boundaries:     []StateBoundary{{Name: "rg-a", ResourceGroups: []string{"rg-a"}}},
		Dependencies:   &DependencyGraph{},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ScanResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SubscriptionID != result.SubscriptionID {
		t.Errorf("SubscriptionID mismatch after round-trip")
	}
}
