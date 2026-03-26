// Package terragrunt generates a DRY Terragrunt project layout from the
// refined Terraform module produced by the refine stage.
//
// Layout produced:
//
//	<outputDir>/
//	  module/                    — the refined TF module (backend.tf / providers.tf excluded)
//	    terraform.tf             — required_version + required_providers
//	    variables.tf             — all input variables incl. resource_group_name, environment
//	    locals.tf                — common_tags (environment key wired to var.environment)
//	    resources.*.tf           — topic resource files
//	  root.hcl                   — remote_state + generate "provider" + global inputs
//	  envs/
//	    <env>/
//	      terragrunt.hcl         — include root.hcl, source = ../../module, env inputs
package terragrunt

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// Options controls Terragrunt layout generation.
type Options struct {
	// OutputDir is the root directory for the Terragrunt layout.
	OutputDir string
	// Environments is the list of deployment tiers (e.g. ["prod", "dev"]).
	// Defaults to ["prod", "dev"] when nil.
	Environments []string
	// SourceResourceGroup is the RG name from the aztfexport run.
	// Used to derive per-environment RG names (e.g. rg-myapp-prod → rg-myapp-dev).
	SourceResourceGroup string
}

// DefaultOptions returns Options with sensible defaults for outputDir.
func DefaultOptions(outputDir string) Options {
	return Options{
		OutputDir:    outputDir,
		Environments: []string{"prod", "dev"},
	}
}

// Run generates the full Terragrunt layout from the refined module files.
func Run(files []*refine.ParsedFile, opts Options) error {
	if len(opts.Environments) == 0 {
		opts.Environments = []string{"prod", "dev"}
	}

	moduleDir := filepath.Join(opts.OutputDir, "module")
	if err := os.MkdirAll(moduleDir, 0o750); err != nil {
		return fmt.Errorf("creating module dir: %w", err)
	}

	// 1. Write module/ — extract RG locals → variables, patch common_tags.
	modResult, err := writeModule(files, moduleDir)
	if err != nil {
		return fmt.Errorf("writing module: %w", err)
	}

	// 2. Write root.hcl.
	if err := writeRoot(opts.OutputDir, modResult.Location); err != nil {
		return fmt.Errorf("writing root.hcl: %w", err)
	}

	// 3. Write envs/<env>/terragrunt.hcl for each environment.
	primaryEnv := opts.Environments[0]
	for _, env := range opts.Environments {
		envDir := filepath.Join(opts.OutputDir, "envs", env)
		if err := os.MkdirAll(envDir, 0o750); err != nil {
			return fmt.Errorf("creating env dir %s: %w", env, err)
		}

		rgInputs := deriveEnvRGInputs(modResult.RGLocals, opts.SourceResourceGroup, primaryEnv, env)
		if err := writeEnvStack(envDir, env, rgInputs); err != nil {
			return fmt.Errorf("writing env stack %s: %w", env, err)
		}
	}

	// 4. Remove the flat .tf files from outputDir root — they are now in
	// module/ and having them in the root would confuse terraform/terragrunt.
	if err := removeRootTFFiles(opts.OutputDir); err != nil {
		return fmt.Errorf("cleaning root .tf files: %w", err)
	}

	return nil
}

// removeRootTFFiles deletes all *.tf files directly inside dir (non-recursive).
// These are the original refine-stage outputs that have been superseded by the
// files written into module/.
func removeRootTFFiles(dir string) error {
	entries, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return err
	}
	for _, path := range entries {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}
