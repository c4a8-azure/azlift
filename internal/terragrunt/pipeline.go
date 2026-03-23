package terragrunt

import (
	"fmt"
	"path/filepath"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// Options controls Terragrunt layout generation.
type Options struct {
	// OutputDir is the root directory for the Terragrunt layout.
	OutputDir string
	// ModulesDir is the relative path (from OutputDir) where module source
	// files live, e.g. "modules". Workload terragrunt.hcl files reference
	// "${get_repo_root()}/<ModulesDir>/<workload>".
	ModulesDir string
	// RootConfig configures the root terragrunt.hcl.
	RootConfig RootConfig
	// Environments is the list of deployment environments to generate.
	// Defaults to prod/staging/dev when nil.
	Environments []Environment
	// ApplyDevSubst enables dev SKU substitution when generating the dev
	// environment inputs from prod values.
	ApplyDevSubst bool
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions(outputDir string) Options {
	return Options{
		OutputDir:     outputDir,
		ModulesDir:    "modules",
		RootConfig:    DefaultRootConfig(),
		Environments:  DefaultEnvironments(),
		ApplyDevSubst: true,
	}
}

// Run generates the full Terragrunt layout from the refined module files:
//
//  1. Write root terragrunt.hcl.
//  2. Derive workloads from grouped .tf file names.
//  3. Write _envcommon/<workload>.hcl for each workload.
//  4. Write <env>/env.hcl + <env>/<workload>/terragrunt.hcl for each env.
func Run(groupedFiles []*refine.ParsedFile, opts Options) error {
	// 1. Root config.
	if err := GenerateRoot(opts.OutputDir, opts.RootConfig); err != nil {
		return fmt.Errorf("generating root terragrunt.hcl: %w", err)
	}

	// 2. Derive workloads.
	names := make([]string, len(groupedFiles))
	for i, pf := range groupedFiles {
		names[i] = filepath.Base(pf.Path)
	}
	modDir := opts.ModulesDir
	if modDir == "" {
		modDir = "modules"
	}
	workloads := DefaultWorkloads(names, modDir)

	// 3. _envcommon.
	if err := GenerateEnvcommon(opts.OutputDir, workloads); err != nil {
		return fmt.Errorf("generating _envcommon: %w", err)
	}

	envs := opts.Environments
	if len(envs) == 0 {
		envs = DefaultEnvironments()
	}

	// Apply dev substitutions to workload inputs for the dev environment.
	if opts.ApplyDevSubst {
		workloads = applyDevToWorkloads(workloads, envs)
	}

	// 4. Per-environment directories.
	if err := GenerateEnvironments(opts.OutputDir, envs, workloads); err != nil {
		return fmt.Errorf("generating environments: %w", err)
	}

	return nil
}

// applyDevToWorkloads applies SKU substitution to workload inputs for any
// environment named "dev" or starting with "dev". Other environments are
// left unchanged (their overrides are picked up at plan time via env.hcl).
func applyDevToWorkloads(workloads []WorkloadInputs, envs []Environment) []WorkloadInputs {
	// Check whether a dev environment exists.
	hasDev := false
	for _, e := range envs {
		if e.Name == "dev" {
			hasDev = true
			break
		}
	}
	if !hasDev {
		return workloads
	}

	result := make([]WorkloadInputs, len(workloads))
	for i, w := range workloads {
		devInputs := ApplyDevSubstitutions(w.Inputs, nil)
		devInputs = DowngradeInstanceCount(devInputs)
		result[i] = WorkloadInputs{
			Name:         w.Name,
			ModuleSource: w.ModuleSource,
			Inputs:       devInputs,
		}
	}
	return result
}
