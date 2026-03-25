package gitrepo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c4a8-azure/azlift/internal/gitrepo"
)

func TestInit_CreatesGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := gitrepo.Init(context.Background(), dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf(".git directory not created: %v", err)
	}
}

func TestCommit_RecordsFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if err := gitrepo.Init(ctx, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := gitrepo.ConfigUser(ctx, dir, "azlift-test", "test@azlift"); err != nil {
		t.Fatalf("ConfigUser: %v", err)
	}

	// Write a file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := gitrepo.Add(ctx, dir, "."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := gitrepo.Commit(ctx, dir, "chore: initial commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestAddRemote_SetsURL(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	if err := gitrepo.Init(ctx, dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := gitrepo.AddRemote(ctx, dir, "origin", "https://github.com/my-org/my-repo.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
}
