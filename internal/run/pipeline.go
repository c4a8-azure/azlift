// Package run implements the full azlift pipeline: SCAN → EXPORT → REFINE → BOOTSTRAP.
// It is the orchestrator behind `azlift run`.
package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/c4a8-azure/azlift/internal/bootstrap"
	"github.com/c4a8-azure/azlift/internal/enrich"
	"github.com/c4a8-azure/azlift/internal/export"
	"github.com/c4a8-azure/azlift/internal/refine"
	"github.com/c4a8-azure/azlift/internal/scan"
	"github.com/c4a8-azure/azlift/internal/terragrunt"
)

// Options drives the full pipeline.
type Options struct {
	// SubscriptionID is the target Azure subscription (required).
	SubscriptionID string
	// ResourceGroup is the Azure resource group to process (required).
	ResourceGroup string
	// RepoName is the Git repository to create during bootstrap.
	RepoName string
	// RepoOrg is the GitHub org or ADO organisation.
	RepoOrg string
	// Platform is "github" or "ado". Defaults to "github".
	Platform string
	// Environments is the list of deployment tiers for bootstrap.
	// Defaults to ["prod", "staging", "dev"].
	Environments []string
	// Location is the Azure region for state storage. Defaults to "westeurope".
	Location string
	// TenantID is the Azure AD tenant (auto-detected if empty).
	TenantID string
	// WorkDir is the base directory for all pipeline outputs.
	// Defaults to ".azlift" in the current directory.
	WorkDir string
	// Mode is "modules" or "terragrunt". Defaults to "modules".
	Mode string
	// Enrich activates the post-refine quality pass.
	Enrich bool
	// FixSecurity auto-remediates safe security anti-patterns.
	FixSecurity bool
	// NoBootstrap skips the bootstrap stage and runs terraform plan instead.
	NoBootstrap bool
	// DryRun prints planned actions without executing any external tools.
	DryRun bool
	// SkipLint skips the tflint pass.
	SkipLint bool
	// SkipDocs skips the terraform-docs pass.
	SkipDocs bool
	// Log is an optional structured logger for progress output.
	Log *slog.Logger
}

// Result summarises the outcome of the full pipeline.
type Result struct {
	// ScanResult is the resource inventory output from SCAN.
	ScanResult *scan.ScanResult
	// RawDir is the path to the raw aztfexport output.
	RawDir string
	// RefinedDir is the path to the refined Terraform output.
	RefinedDir string
	// RefineResult is the REFINE stage output.
	RefineResult refine.Result
	// BootstrapResult is the BOOTSTRAP stage output (nil when --no-bootstrap).
	BootstrapResult *bootstrap.Result
	// TerraformPlanOutput is the terraform plan output when --no-bootstrap.
	TerraformPlanOutput string
}

// Run executes the full pipeline in sequence:
//
//  1. SCAN   — inventory the Azure subscription
//  2. EXPORT — export the resource group via aztfexport
//  3. REFINE — transform raw HCL into structured Terraform/Terragrunt
//  4. BOOTSTRAP (or terraform plan if --no-bootstrap)
//
// When --dry-run is set no external tools are invoked; the function
// returns after printing the planned actions to the logger.
func Run(ctx context.Context, opts Options) (Result, error) {
	var result Result
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	opts = applyDefaults(opts)

	if opts.DryRun {
		printDryRunPlan(log, opts)
		return result, nil
	}

	rawDir := filepath.Join(opts.WorkDir, "raw", opts.ResourceGroup)
	refinedDir := filepath.Join(opts.WorkDir, "refined")
	result.RawDir = rawDir
	result.RefinedDir = refinedDir

	// ── Stage 1: SCAN ──────────────────────────────────────────────────────
	log.Info("[SCAN] querying resource inventory", "subscription", opts.SubscriptionID)
	scanResult, err := runScan(ctx, log, opts)
	if err != nil {
		return result, fmt.Errorf("scan: %w", err)
	}
	result.ScanResult = scanResult

	// Save scan result to work dir.
	if _, err := scan.SaveResult(scanResult, opts.WorkDir); err != nil {
		return result, fmt.Errorf("saving scan result: %w", err)
	}
	log.Info("[SCAN] complete", "resource_groups", len(scanResult.ResourceGroups))

	// ── Stage 2: EXPORT ────────────────────────────────────────────────────
	log.Info("[EXPORT] exporting resource group via aztfexport", "resource_group", opts.ResourceGroup)
	if err := runExport(ctx, log, opts, rawDir); err != nil {
		return result, fmt.Errorf("export: %w", err)
	}
	log.Info("[EXPORT] complete", "output_dir", rawDir)

	// ── Stage 3: REFINE ────────────────────────────────────────────────────
	log.Info("[REFINE] transforming HCL", "mode", opts.Mode)
	refineResult, err := runRefine(ctx, log, opts, rawDir, refinedDir)
	if err != nil {
		return result, fmt.Errorf("refine: %w", err)
	}
	result.RefineResult = refineResult
	log.Info("[REFINE] complete", "files", len(refineResult.Files))

	// ── Stage 4: BOOTSTRAP or terraform plan ───────────────────────────────
	if opts.NoBootstrap {
		log.Info("[PLAN] running terraform plan (--no-bootstrap)")
		planOut, err := runTerraformPlan(ctx, log, refinedDir)
		if err != nil {
			// Plan failure is reported but not fatal for the pipeline result.
			log.Info("[PLAN] terraform plan reported issues", "err", err)
		}
		result.TerraformPlanOutput = planOut
		log.Info("[PLAN] complete")
	} else {
		if opts.RepoName == "" {
			return result, fmt.Errorf("--repo-name is required for bootstrap; use --no-bootstrap to skip")
		}
		log.Info("[BOOTSTRAP] provisioning CI/CD infrastructure", "repo", opts.RepoName, "platform", opts.Platform)
		bResult, err := runBootstrap(ctx, log, opts, refinedDir)
		if err != nil {
			return result, fmt.Errorf("bootstrap: %w", err)
		}
		result.BootstrapResult = &bResult
		log.Info("[BOOTSTRAP] complete")
	}

	return result, nil
}

// ── private helpers ──────────────────────────────────────────────────────────

func applyDefaults(opts Options) Options {
	if opts.Platform == "" {
		opts.Platform = "github"
	}
	if opts.Mode == "" {
		opts.Mode = "modules"
	}
	if opts.Location == "" {
		opts.Location = "westeurope"
	}
	if len(opts.Environments) == 0 {
		opts.Environments = []string{"prod", "staging", "dev"}
	}
	if opts.WorkDir == "" {
		opts.WorkDir = ".azlift"
	}
	return opts
}

func runScan(ctx context.Context, log *slog.Logger, opts Options) (*scan.ScanResult, error) {
	client, err := scan.NewClient()
	if err != nil {
		return nil, fmt.Errorf("building Resource Graph client: %w", err)
	}

	log.Info("[SCAN] querying inventory")
	groups, err := scan.Inventory(ctx, client, opts.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("inventory query: %w", err)
	}

	graph := scan.AnalyseDependencies(groups)
	return scan.BuildResult(opts.SubscriptionID, groups, graph), nil
}

func runExport(ctx context.Context, log *slog.Logger, opts Options, rawDir string) error {
	if err := os.MkdirAll(rawDir, 0o750); err != nil {
		return fmt.Errorf("creating raw dir: %w", err)
	}

	baseRunner := &export.AztfexportRunner{}
	runner := export.NewRetryRunner(baseRunner)

	args := []string{
		"resource-group",
		"--subscription-id", opts.SubscriptionID,
		"--output-dir", rawDir,
		"--non-interactive",
		opts.ResourceGroup,
	}

	if err := runner.Run(ctx, args, func(line string) { log.Debug("[aztfexport] " + line) }); err != nil {
		return err
	}

	manifest := &export.Manifest{
		SchemaVersion:  "1",
		SubscriptionID: opts.SubscriptionID,
		ResourceGroup:  opts.ResourceGroup,
		OutputDir:      rawDir,
	}
	if _, err := export.WriteManifest(manifest, rawDir); err != nil {
		return err
	}

	return nil
}

func runRefine(ctx context.Context, log *slog.Logger, opts Options, rawDir, refinedDir string) (refine.Result, error) {
	result, err := refine.Run(ctx, refine.Options{
		InputDir:      rawDir,
		OutputDir:     refinedDir,
		ResourceGroup: opts.ResourceGroup,
		SkipLint:      opts.SkipLint || opts.Mode == "terragrunt",
		SkipDocs:      opts.SkipDocs,
	})
	if err != nil {
		return result, err
	}

	doEnrich := opts.Enrich || opts.FixSecurity
	if doEnrich {
		log.Info("[REFINE] running enrichment pass")
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		var localsFile *refine.ParsedFile
		for _, pf := range result.Files {
			if len(pf.Path) >= 9 && pf.Path[len(pf.Path)-9:] == "locals.tf" {
				localsFile = pf
				break
			}
		}
		if _, err := enrich.Run(ctx, result.Files, localsFile, enrich.Options{
			APIKey:      apiKey,
			FixSecurity: opts.FixSecurity,
			Log:         log,
		}); err != nil {
			return result, fmt.Errorf("enrichment: %w", err)
		}
		for _, pf := range result.Files {
			if err := refine.WriteFile(pf); err != nil {
				return result, fmt.Errorf("writing enriched %s: %w", pf.Path, err)
			}
		}
	}

	if opts.Mode == "terragrunt" {
		log.Info("[REFINE] generating Terragrunt layout")
		tgOpts := terragrunt.DefaultOptions(refinedDir)
		if err := terragrunt.Run(result.Files, tgOpts); err != nil {
			return result, fmt.Errorf("terragrunt layout: %w", err)
		}
	}

	return result, nil
}

func runBootstrap(ctx context.Context, _ *slog.Logger, opts Options, refinedDir string) (bootstrap.Result, error) {
	return bootstrap.Run(ctx, bootstrap.Options{
		SubscriptionID: opts.SubscriptionID,
		TenantID:       opts.TenantID,
		RepoName:       opts.RepoName,
		RepoOrg:        opts.RepoOrg,
		Platform:       opts.Platform,
		Environments:   opts.Environments,
		InputDir:       refinedDir,
		Location:       opts.Location,
		Log:            opts.Log,
	})
}

func runTerraformPlan(ctx context.Context, log *slog.Logger, dir string) (string, error) {
	runner := &tfPlanRunner{}
	return runner.Plan(ctx, log, dir)
}

// printDryRunPlan logs the actions that would be taken without executing them.
func printDryRunPlan(log *slog.Logger, opts Options) {
	log.Info("[DRY-RUN] planned pipeline actions")
	log.Info("[DRY-RUN]   1. SCAN   — query Azure Resource Graph",
		"subscription", opts.SubscriptionID)
	log.Info("[DRY-RUN]   2. EXPORT — run aztfexport",
		"resource_group", opts.ResourceGroup,
		"output_dir", filepath.Join(opts.WorkDir, "raw", opts.ResourceGroup))
	log.Info("[DRY-RUN]   3. REFINE — transform HCL",
		"mode", opts.Mode,
		"enrich", opts.Enrich,
		"output_dir", filepath.Join(opts.WorkDir, "refined"))
	if opts.NoBootstrap {
		log.Info("[DRY-RUN]   4. PLAN   — run terraform plan (--no-bootstrap)")
	} else {
		log.Info("[DRY-RUN]   4. BOOTSTRAP — provision state, MIs, CI/CD platform",
			"repo", opts.RepoOrg+"/"+opts.RepoName,
			"platform", opts.Platform)
	}
	log.Info("[DRY-RUN] no actions executed")
}
