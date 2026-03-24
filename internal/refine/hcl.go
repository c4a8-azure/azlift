// Package refine implements Stage 3 of the azlift pipeline: multi-step HCL
// transformation (variable extraction, semantic analysis, resource grouping,
// backend/provider generation). Supports --mode modules and --mode terragrunt.
package refine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ParsedFile holds a parsed HCL file alongside its origin path.
type ParsedFile struct {
	// Path is the absolute path of the source file.
	Path string
	// File is the writable AST representation.
	File *hclwrite.File
}

// ParseDir reads every *.tf file in dir and returns a ParsedFile per file.
// Files with HCL syntax errors are collected into a combined error so the
// caller sees all problems at once rather than stopping at the first.
func ParseDir(dir string) ([]*ParsedFile, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, fmt.Errorf("listing .tf files in %s: %w", dir, err)
	}

	var files []*ParsedFile
	var errs []string

	for _, path := range entries {
		pf, err := ParseFile(path)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		files = append(files, pf)
	}

	if len(errs) > 0 {
		return files, fmt.Errorf("HCL parse errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return files, nil
}

// ParseFile reads and parses a single .tf file.
func ParseFile(path string) (*ParsedFile, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path comes from our own directory listing
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	f, diags := hclwrite.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing %s: %s", path, diags.Error())
	}

	return &ParsedFile{Path: path, File: f}, nil
}

// WriteFile serialises pf back to its source path, preserving the file mode.
func WriteFile(pf *ParsedFile) error {
	return WriteFileTo(pf, pf.Path)
}

// WriteFileTo serialises pf to an explicit destination path.
func WriteFileTo(pf *ParsedFile, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}
	return os.WriteFile(dest, pf.File.Bytes(), 0o600)
}

// WriteDir serialises all parsed files into destDir, using each file's
// base name as the output filename.
func WriteDir(files []*ParsedFile, destDir string) error {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("creating output dir %s: %w", destDir, err)
	}
	for _, pf := range files {
		dest := filepath.Join(destDir, filepath.Base(pf.Path))
		if err := WriteFileTo(pf, dest); err != nil {
			return err
		}
	}
	return nil
}

// Blocks returns all top-level blocks in pf matching the given blockType
// (e.g. "resource", "variable", "locals").
func Blocks(pf *ParsedFile, blockType string) []*hclwrite.Block {
	var out []*hclwrite.Block
	for _, block := range pf.File.Body().Blocks() {
		if block.Type() == blockType {
			out = append(out, block)
		}
	}
	return out
}

// AllBlocks collects blocks of blockType across all parsed files.
func AllBlocks(files []*ParsedFile, blockType string) []*hclwrite.Block {
	var out []*hclwrite.Block
	for _, pf := range files {
		out = append(out, Blocks(pf, blockType)...)
	}
	return out
}

// NewFile creates an empty ParsedFile at the given path (file not yet written).
func NewFile(path string) *ParsedFile {
	return &ParsedFile{Path: path, File: hclwrite.NewEmptyFile()}
}

// ReplaceContent re-parses src and replaces pf's in-memory AST with the
// result. Used by the AI enrichment pass to update files after the model
// returns modified HCL.
func ReplaceContent(pf *ParsedFile, src []byte) error {
	f, diags := hclwrite.ParseConfig(src, pf.Path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("re-parsing enriched content for %s: %s", pf.Path, diags.Error())
	}
	pf.File = f
	return nil
}
