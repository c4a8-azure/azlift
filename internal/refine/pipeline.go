package refine

import (
	"context"
	"fmt"
	"path/filepath"
)

// Options controls the refine pipeline behaviour.
type Options struct {
	// InputDir is where raw aztfexport .tf files live.
	InputDir string
	// OutputDir is where the refined files will be written.
	OutputDir string
	// ResourceGroup is used to derive the backend state key.
	ResourceGroup string
	// SkipLint bypasses the tflint pass when true.
	SkipLint bool
	// SkipDocs bypasses terraform-docs generation when true.
	SkipDocs bool
	// LintRunner overrides the default ExecTflintRunner (used in tests).
	LintRunner TflintRunner
	// DocsRunner overrides the default ExecTerraformDocsRunner (used in tests).
	DocsRunner TerraformDocsRunner
}

// StageLabel is the log prefix used for the REFINE stage.
const StageLabel = "REFINE"

// Result summarises what the pipeline produced.
type Result struct {
	// Files is the set of generated ParsedFiles (relative to OutputDir).
	Files []*ParsedFile
	// Lint holds the outcome of the tflint pass.
	Lint LintResult
	// Docs holds the outcome of the terraform-docs pass.
	Docs DocsResult
}

// Run executes the full refine pipeline in modules mode:
//
//  1. Parse all .tf files in InputDir.
//  2. Extract repeated literals into variables.tf / locals.tf.
//  3. Group resource blocks into topic files.
//  4. Generate backend.tf, terraform.tf, providers.tf.
//  5. Write all files to OutputDir.
//  6. Run tflint (unless SkipLint).
//  7. Run terraform-docs (unless SkipDocs).
func Run(ctx context.Context, opts Options) (Result, error) {
	var result Result

	// 1. Parse input.
	files, err := ParseDir(opts.InputDir)
	if err != nil {
		return result, fmt.Errorf("parsing input: %w", err)
	}

	// 2. Variable extraction.
	varsFile, localsFile, err := ExtractVariables(files, opts.OutputDir)
	if err != nil {
		return result, fmt.Errorf("extracting variables: %w", err)
	}

	// 3. Group resource blocks into topic files.
	grouped := GroupResources(files, opts.OutputDir)

	// 4. Scaffold files.
	rg := opts.ResourceGroup
	if rg == "" {
		rg = filepath.Base(opts.InputDir)
	}
	backendCfg := DefaultBackendConfig(rg)
	backendFile, err := GenerateBackend(opts.OutputDir, backendCfg)
	if err != nil {
		return result, fmt.Errorf("generating backend.tf: %w", err)
	}

	versionsFile, err := GenerateVersions(opts.OutputDir, "", nil)
	if err != nil {
		return result, fmt.Errorf("generating terraform.tf: %w", err)
	}

	providersFile, err := GenerateProvider(opts.OutputDir)
	if err != nil {
		return result, fmt.Errorf("generating providers.tf: %w", err)
	}

	// 5. Write all files to OutputDir.
	allFiles := make([]*ParsedFile, 0, len(grouped)+5)
	allFiles = append(allFiles, grouped...)
	allFiles = append(allFiles, varsFile, localsFile, backendFile, versionsFile, providersFile)

	for _, pf := range allFiles {
		if err := WriteFile(pf); err != nil {
			return result, fmt.Errorf("writing %s: %w", pf.Path, err)
		}
	}
	result.Files = allFiles

	// 6. tflint.
	lintRunner := opts.LintRunner
	if lintRunner == nil {
		lintRunner = &ExecTflintRunner{}
	}
	lintResult, err := RunLint(ctx, lintRunner, opts.OutputDir, opts.SkipLint)
	if err != nil {
		return result, fmt.Errorf("lint: %w", err)
	}
	result.Lint = lintResult

	// 7. terraform-docs.
	docsRunner := opts.DocsRunner
	if docsRunner == nil {
		docsRunner = &ExecTerraformDocsRunner{}
	}
	docsResult, err := RunDocs(ctx, docsRunner, opts.OutputDir, opts.SkipDocs)
	if err != nil {
		// Docs failure is non-fatal — warn but continue.
		docsResult.Output = fmt.Sprintf("terraform-docs warning: %v", err)
	}
	result.Docs = docsResult

	return result, nil
}
