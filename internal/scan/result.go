package scan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	ResultSchemaVersion = "1"
	ResultFileName      = "scan-result.json"
)

// ScanResult is the versioned output contract written by the SCAN stage and
// consumed by the EXPORT stage. Its schema version is incremented whenever
// fields are removed or renamed.
type ScanResult struct {
	SchemaVersion  string           `json:"schemaVersion"`
	ScannedAt      time.Time        `json:"scannedAt"`
	SubscriptionID string           `json:"subscriptionId"`
	ResourceGroups []*ResourceGroup `json:"resourceGroups"`
	Dependencies   *DependencyGraph `json:"dependencies"`
	Boundaries     []StateBoundary  `json:"stateBoundaries"`
}

// StateBoundary represents one recommended Terraform root — a set of resource
// groups that should share a state file.
type StateBoundary struct {
	// Name is a human-readable label for the boundary (e.g. "networking").
	Name string `json:"name"`
	// ResourceGroups lists the RGs that belong to this Terraform root.
	ResourceGroups []string `json:"resourceGroups"`
	// DependsOn names other boundaries whose outputs this one references.
	DependsOn []string `json:"dependsOn"`
	// Reason explains why these RGs were grouped together.
	Reason string `json:"reason"`
}

// BuildResult assembles a ScanResult from inventory and dependency data.
func BuildResult(subscriptionID string, groups map[string]*ResourceGroup, graph *DependencyGraph) *ScanResult {
	rgs := make([]*ResourceGroup, 0, len(groups))
	for _, name := range sortedKeys(groups) {
		rgs = append(rgs, groups[name])
	}

	boundaries := recommendBoundaries(groups, graph)

	return &ScanResult{
		SchemaVersion:  ResultSchemaVersion,
		ScannedAt:      time.Now().UTC(),
		SubscriptionID: subscriptionID,
		ResourceGroups: rgs,
		Dependencies:   graph,
		Boundaries:     boundaries,
	}
}

// SaveResult writes result as indented JSON to outputDir/scan-result.json.
func SaveResult(result *ScanResult, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	path := filepath.Join(outputDir, ResultFileName)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialising scan result: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing scan result: %w", err)
	}
	return path, nil
}

// PrintRecommendations writes a human-readable boundary recommendation to w.
func PrintRecommendations(w io.Writer, result *ScanResult) {
	_, _ = fmt.Fprintf(w, "\nState boundary recommendations (%d Terraform root(s)):\n\n", len(result.Boundaries))
	for i, b := range result.Boundaries {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, b.Name)
		_, _ = fmt.Fprintf(w, "     Resource groups: %v\n", b.ResourceGroups)
		if len(b.DependsOn) > 0 {
			_, _ = fmt.Fprintf(w, "     Depends on:      %v\n", b.DependsOn)
		}
		_, _ = fmt.Fprintf(w, "     Reason:          %s\n\n", b.Reason)
	}
	if len(result.Dependencies.Edges) > 0 {
		_, _ = fmt.Fprintln(w, "Cross-root dependencies detected:")
		for _, e := range result.Dependencies.Edges {
			_, _ = fmt.Fprintf(w, "  %s → %s  (%s)\n", e.From, e.To, e.Reason)
		}
		_, _ = fmt.Fprintln(w)
	}
}

// recommendBoundaries groups resource groups into Terraform state roots.
// Strategy: each RG starts as its own boundary; RGs with no cross-RG
// dependencies remain independent; RGs referenced by others are noted as
// upstream dependencies.
func recommendBoundaries(groups map[string]*ResourceGroup, graph *DependencyGraph) []StateBoundary {
	// Build a set of RGs that are depended upon by others.
	dependedUpon := map[string]struct{}{}
	for _, e := range graph.Edges {
		dependedUpon[e.To] = struct{}{}
	}

	names := sortedKeys(groups)
	boundaries := make([]StateBoundary, 0, len(names))

	for _, name := range names {
		deps := graph.DependsOn(name)
		sort.Strings(deps)

		reason := "independent resource group — no cross-RG dependencies"
		if len(deps) > 0 {
			reason = fmt.Sprintf("references resources in: %v", deps)
		}
		if _, ok := dependedUpon[name]; ok && len(deps) == 0 {
			reason = "upstream dependency — referenced by other resource groups"
		}

		boundaries = append(boundaries, StateBoundary{
			Name:           name,
			ResourceGroups: []string{name},
			DependsOn:      deps,
			Reason:         reason,
		})
	}

	return boundaries
}
