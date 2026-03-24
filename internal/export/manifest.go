package export

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const manifestFileName = "export-manifest.json"

// Manifest records metadata about one export run for a single resource group.
// It is written alongside the raw .tf files and consumed by the REFINE stage.
type Manifest struct {
	SchemaVersion     string    `json:"schemaVersion"`
	ExportedAt        time.Time `json:"exportedAt"`
	SubscriptionID    string    `json:"subscriptionId"`
	ResourceGroup     string    `json:"resourceGroup"`
	OutputDir         string    `json:"outputDir"`
	ExcludedResources []string  `json:"excludedResources"`
	DataStubs         []string  `json:"dataStubs"`
	AztfexportVersion string    `json:"aztfexportVersion,omitempty"`
}

// ExportOutput defines the per-RG output directory layout:
//
//	<outputDir>/
//	└── <resourceGroup>/
//	    ├── main.tf
//	    ├── terraform.tfstate
//	    └── export-manifest.json
type ExportOutput struct {
	// RGDir is the full path to the resource-group subdirectory.
	RGDir string
	// ManifestPath is the path of the written manifest file.
	ManifestPath string
}

// PrepareOutputDir creates the per-RG subdirectory and returns its path.
// If the directory already exists and is non-empty it is removed and
// recreated — aztfexport refuses to write into a non-empty directory,
// and the raw export is always regenerated fresh from Azure.
func PrepareOutputDir(baseDir, resourceGroup string) (string, error) {
	rgDir := filepath.Join(baseDir, resourceGroup)

	if entries, err := os.ReadDir(rgDir); err == nil && len(entries) > 0 {
		slog.Warn("[EXPORT] output directory is not empty — clearing for fresh export", "dir", rgDir)
		if err := os.RemoveAll(rgDir); err != nil {
			return "", fmt.Errorf("clearing stale output dir %s: %w", rgDir, err)
		}
	}

	if err := os.MkdirAll(rgDir, 0o750); err != nil {
		return "", fmt.Errorf("creating output dir %s: %w", rgDir, err)
	}
	return rgDir, nil
}

// WriteManifest serialises m to <rgDir>/export-manifest.json.
func WriteManifest(m *Manifest, rgDir string) (string, error) {
	path := filepath.Join(rgDir, manifestFileName)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialising manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing manifest: %w", err)
	}
	return path, nil
}

// ReadManifest loads a manifest from path.
func ReadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from our own output structure
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}
	return &m, nil
}
