package refine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options controls the refine pipeline behaviour.
type Options struct {
	// InputDir is where raw aztfexport .tf files live.
	InputDir string
	// OutputDir is where the refined files will be written.
	OutputDir string
	// ResourceGroup is used to derive the backend state key.
	ResourceGroup string
	// MinTerraformVersion overrides the required_version injected into
	// terraform.tf when the aztfexport input omits it. Defaults to ">= 1.10".
	MinTerraformVersion string
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
	// StateCopied is true when terraform.tfstate was found in InputDir and
	// copied to OutputDir so the bootstrap stage can locate it there.
	StateCopied bool
}

// Run executes the full refine pipeline in modules mode:
//
//  1. Parse all .tf files in InputDir.
//  2. Extract repeated literals into variables.tf / locals.tf.
//  3. Normalise tags — inject common_tags local and rewrite resource tags to merge().
//  4. Group resource blocks into topic files.
//  5. Generate backend.tf, terraform.tf, providers.tf.
//  6. Write all files to OutputDir.
//  7. Copy terraform.tfstate from InputDir → OutputDir (if present).
//  8. Run tflint (unless SkipLint).
//  9. Run terraform-docs (unless SkipDocs).
func Run(ctx context.Context, opts Options) (Result, error) {
	var result Result

	// 1. Parse input.
	files, err := ParseDir(opts.InputDir)
	if err != nil {
		return result, fmt.Errorf("parsing input: %w", err)
	}
	if len(files) == 0 {
		if hint := exportParentHint(opts.InputDir); hint != "" {
			return result, fmt.Errorf(
				"no .tf files found in %s\n\n"+
					"hint: %s\n"+
					"      pass one of those as --input-dir instead",
				opts.InputDir, hint,
			)
		}
		return result, fmt.Errorf("no .tf files found in %s", opts.InputDir)
	}

	// 2. Variable extraction.
	varsFile, localsFile, err := ExtractVariables(files, opts.OutputDir)
	if err != nil {
		return result, fmt.Errorf("extracting variables: %w", err)
	}

	// 3. Tag normalisation — always runs so every resource gets
	// merge(local.common_tags, {...}) regardless of whether --enrich is used.
	NormaliseTags(files, localsFile)

	// 4. Group resource blocks into topic files.
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

	versionsFile, err := ExtractTerraformBlock(opts.OutputDir, files, opts.MinTerraformVersion)
	if err != nil {
		return result, fmt.Errorf("extracting terraform.tf: %w", err)
	}
	if versionsFile == nil {
		versionsFile, err = GenerateVersions(opts.OutputDir, opts.MinTerraformVersion, nil)
		if err != nil {
			return result, fmt.Errorf("generating terraform.tf: %w", err)
		}
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

	// 6. Carry terraform.tfstate forward from the export dir so the bootstrap
	// stage can locate it at OutputDir/terraform.tfstate without needing an
	// explicit --state-dir flag.
	srcState := filepath.Join(opts.InputDir, "terraform.tfstate")
	dstState := filepath.Join(opts.OutputDir, "terraform.tfstate")
	if copied, copyErr := copyFileIfExists(srcState, dstState); copyErr != nil {
		return result, fmt.Errorf("copying terraform.tfstate: %w", copyErr)
	} else {
		result.StateCopied = copied
	}

	// 8. tflint.
	lintRunner := opts.LintRunner
	if lintRunner == nil {
		lintRunner = &ExecTflintRunner{}
	}
	lintResult, err := RunLint(ctx, lintRunner, opts.OutputDir, opts.SkipLint)
	if err != nil {
		return result, fmt.Errorf("lint: %w", err)
	}
	result.Lint = lintResult

	// 9. terraform-docs.
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

// copyFileIfExists copies src to dst when src exists. Returns (true, nil) on
// success, (false, nil) when src is absent, or (false, err) on failure.
func copyFileIfExists(src, dst string) (bool, error) {
	in, err := os.Open(src) //nolint:gosec
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer in.Close()

	out, err := os.Create(dst) //nolint:gosec
	if err != nil {
		return false, err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return false, err
	}
	return true, nil
}

// exportParentHint checks whether dir looks like an aztfexport parent directory
// (i.e. it contains subdirectories that themselves hold .tf files). If so it
// returns a human-readable string listing the candidate subdirectories so the
// user can pick the right --input-dir. Returns "" when no such layout is found.
func exportParentHint(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		tfs, _ := filepath.Glob(filepath.Join(sub, "*.tf"))
		if len(tfs) > 0 {
			candidates = append(candidates, sub)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return "looks like an export parent directory; found .tf files in:\n      " +
		strings.Join(candidates, "\n      ")
}
