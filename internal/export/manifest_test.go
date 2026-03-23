package export

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareOutputDir_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	rgDir, err := PrepareOutputDir(base, "rg-prod-network")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(rgDir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
	if filepath.Base(rgDir) != "rg-prod-network" {
		t.Errorf("unexpected dir name: %s", filepath.Base(rgDir))
	}
}

func TestPrepareOutputDir_Idempotent(t *testing.T) {
	base := t.TempDir()
	if _, err := PrepareOutputDir(base, "rg-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareOutputDir(base, "rg-a"); err != nil {
		t.Fatal("second call should be idempotent")
	}
}

func TestWriteManifest_RoundTrip(t *testing.T) {
	base := t.TempDir()
	m := &Manifest{
		SchemaVersion:     "1",
		ExportedAt:        time.Now().UTC().Truncate(time.Second),
		SubscriptionID:    "sub-123",
		ResourceGroup:     "rg-prod",
		OutputDir:         base,
		ExcludedResources: []string{"diag-settings-1"},
		DataStubs:         []string{"data_bastion.tf"},
	}
	path, err := WriteManifest(m, base)
	if err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if got.ResourceGroup != m.ResourceGroup {
		t.Errorf("ResourceGroup: want %s, got %s", m.ResourceGroup, got.ResourceGroup)
	}
	if len(got.ExcludedResources) != 1 {
		t.Errorf("ExcludedResources: want 1, got %d", len(got.ExcludedResources))
	}
	if len(got.DataStubs) != 1 {
		t.Errorf("DataStubs: want 1, got %d", len(got.DataStubs))
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	_, err := ReadManifest("/nonexistent/path/manifest.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestWriteManifest_FilePermissions(t *testing.T) {
	base := t.TempDir()
	m := &Manifest{SchemaVersion: "1", ResourceGroup: "rg"}
	path, err := WriteManifest(m, base)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("manifest permissions: want 0600, got %04o", info.Mode().Perm())
	}
}
