// Package run orchestrates the full azlift pipeline end-to-end:
// SCAN → EXPORT → REFINE → REPO INIT → GITHUB → ACTIVATE
package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/c4a8-azure/azlift/internal/bootstrap"
	"github.com/c4a8-azure/azlift/internal/enrich"
	"github.com/c4a8-azure/azlift/internal/export"
	"github.com/c4a8-azure/azlift/internal/refine"
	"github.com/c4a8-azure/azlift/internal/scan"
	"github.com/c4a8-azure/azlift/internal/terragrunt"
)

// Options drives the full pipeline.
type Options struct {
	// ─── Source ──────────────────────────────────────────────────────────────
	// SubscriptionID is the Azure subscription to scan and export from.
	SubscriptionID string
	// ResourceGroup is the primary resource group to export (required).
	ResourceGroup string
	// ResourceGroups are all RGs in scope for RBAC assignments.
	// Defaults to [ResourceGroup] when empty.
	ResourceGroups []string

	// ─── Target (bootstrap) ──────────────────────────────────────────────────
	// TargetSubscription is where CI/CD resources are provisioned.
	// Defaults to SubscriptionID (same-tenant mode).
	TargetSubscription string
	// TargetTenant triggers cross-tenant mode when it differs from TenantID.
	TargetTenant string
	// TenantID is the source Azure AD tenant (auto-detected when empty).
	TenantID string

	// ─── Repository ──────────────────────────────────────────────────────────
	RepoName        string
	RepoOrg         string
	MIResourceGroup string

	// ─── Output ──────────────────────────────────────────────────────────────
	// Mode is "modules" (default) or "terragrunt".
	Mode         string
	Environments []string
	Location     string
	// WorkDir is the base directory for all pipeline outputs (.azlift by default).
	WorkDir      string
	WorkflowsDir string

	// ─── Enrichment ──────────────────────────────────────────────────────────
	Enrich      bool
	FixSecurity bool

	// ─── Misc ────────────────────────────────────────────────────────────────
	DryRun   bool
	SkipLint bool
	SkipDocs bool

	Log *slog.Logger
}

// Result summarises what the pipeline produced.
type Result struct {
	ScanResult      *scan.ScanResult
	RawDir          string
	RefinedDir      string
	RefineResult    refine.Result
	BootstrapResult *bootstrap.Result
}

// Run executes all pipeline stages in order.
func Run(ctx context.Context, opts Options) (Result, error) {
	opts = applyDefaults(opts)
	var result Result

	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	if opts.DryRun {
		printDryRunPlan(log, opts)
		return result, nil
	}

	// ── Stage 1: SCAN ─────────────────────────────────────────────────────────

	log.Info("[SCAN] building Resource Graph client")
	scanClient, err := scan.NewClient()
	if err != nil {
		return result, fmt.Errorf("scan: building client: %w", err)
	}

	log.Info("[SCAN] querying inventory", "subscription", opts.SubscriptionID)
	groups, err := scan.Inventory(ctx, scanClient, opts.SubscriptionID)
	if err != nil {
		return result, fmt.Errorf("scan: inventory: %w", err)
	}
	log.Info(fmt.Sprintf("[SCAN] %d resource group(s) found", len(groups)))

	graph := scan.AnalyseDependencies(groups)
	scanResult := scan.BuildResult(opts.SubscriptionID, groups, graph)
	result.ScanResult = scanResult

	if _, err := scan.SaveResult(scanResult, opts.WorkDir); err != nil {
		return result, fmt.Errorf("scan: saving result: %w", err)
	}

	// ── Stage 2: EXPORT ───────────────────────────────────────────────────────

	rawDir := filepath.Join(opts.WorkDir, "raw")
	result.RawDir = rawDir

	rgDir, err := export.PrepareOutputDir(rawDir, opts.ResourceGroup)
	if err != nil {
		return result, fmt.Errorf("export: prepare output dir: %w", err)
	}

	log.Info("[EXPORT] running aztfexport", "resource_group", opts.ResourceGroup, "output_dir", rgDir)
	exportRunner := export.NewRetryRunner(&export.AztfexportRunner{})
	exportArgs := []string{
		"resource-group",
		"--subscription-id", opts.SubscriptionID,
		"--output-dir", rgDir,
		"--non-interactive",
		opts.ResourceGroup,
	}
	if err := exportRunner.Run(ctx, exportArgs, func(line string) { log.Debug(line) }); err != nil {
		return result, fmt.Errorf("export: aztfexport: %w", err)
	}
	log.Info("[EXPORT] complete")

	manifest := &export.Manifest{
		SchemaVersion:  "1",
		SubscriptionID: opts.SubscriptionID,
		ResourceGroup:  opts.ResourceGroup,
		OutputDir:      rgDir,
	}
	if _, err := export.WriteManifest(manifest, rgDir); err != nil {
		return result, fmt.Errorf("export: writing manifest: %w", err)
	}

	// ── Stage 3: REFINE ───────────────────────────────────────────────────────

	refinedDir := filepath.Join(opts.WorkDir, "refined")
	result.RefinedDir = refinedDir

	log.Info("[REFINE] transforming HCL", "input", rgDir, "output", refinedDir)
	refineResult, err := refine.Run(ctx, refine.Options{
		InputDir:      rgDir,
		OutputDir:     refinedDir,
		ResourceGroup: opts.ResourceGroup,
		SkipLint:      opts.SkipLint || opts.Mode == "terragrunt",
		SkipDocs:      opts.SkipDocs,
	})
	if err != nil {
		return result, fmt.Errorf("refine: %w", err)
	}
	result.RefineResult = refineResult
	log.Info(fmt.Sprintf("[REFINE] %d file(s) written", len(refineResult.Files)))

	// Enrich (optional).
	if opts.Enrich || opts.FixSecurity {
		log.Info("[REFINE] running enrichment pass")
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" && opts.Enrich {
			log.Info("[REFINE] ANTHROPIC_API_KEY not set — AI descriptions skipped")
		}

		var localsFile *refine.ParsedFile
		for _, pf := range refineResult.Files {
			if strings.HasSuffix(pf.Path, "locals.tf") {
				localsFile = pf
				break
			}
		}

		if _, err := enrich.Run(ctx, refineResult.Files, localsFile, enrich.Options{
			APIKey:      apiKey,
			FixSecurity: opts.FixSecurity,
			Log:         log,
		}); err != nil {
			return result, fmt.Errorf("enrich: %w", err)
		}

		for _, pf := range refineResult.Files {
			if err := refine.WriteFile(pf); err != nil {
				return result, fmt.Errorf("refine: writing enriched file: %w", err)
			}
		}
		if localsFile != nil {
			if err := refine.WriteFile(localsFile); err != nil {
				return result, fmt.Errorf("refine: writing enriched locals.tf: %w", err)
			}
		}
		log.Info("[REFINE] enrichment complete")
	}

	// Terragrunt layout (optional).
	if opts.Mode == "terragrunt" {
		log.Info("[REFINE] generating Terragrunt layout")
		if err := terragrunt.Run(refineResult.Files, terragrunt.DefaultOptions(refinedDir)); err != nil {
			return result, fmt.Errorf("terragrunt layout: %w", err)
		}
		log.Info("[REFINE] Terragrunt layout written")
	}

	// ── Stages 4–6: BOOTSTRAP (REPO INIT → GITHUB → ACTIVATE) ───────────────

	if opts.RepoName == "" || opts.RepoOrg == "" {
		log.Info("[BOOTSTRAP] skipped — --repo-name and --org are required")
		return result, nil
	}

	rgScope := opts.ResourceGroups
	if len(rgScope) == 0 {
		rgScope = []string{opts.ResourceGroup}
	}

	log.Info("[BOOTSTRAP] starting", "org", opts.RepoOrg, "repo", opts.RepoName)
	bootstrapResult, err := bootstrap.Run(ctx, bootstrap.Options{
		SubscriptionID:     opts.SubscriptionID,
		TargetSubscription: opts.TargetSubscription,
		TargetTenant:       opts.TargetTenant,
		TenantID:           opts.TenantID,
		RepoName:           opts.RepoName,
		RepoOrg:            opts.RepoOrg,
		Environments:       opts.Environments,
		InputDir:           refinedDir,
		Location:           opts.Location,
		ResourceGroups:     rgScope,
		MIResourceGroup:    opts.MIResourceGroup,
		WorkflowsDir:       opts.WorkflowsDir,
		Log:                log,
	})
	if err != nil {
		return result, fmt.Errorf("bootstrap: %w", err)
	}
	result.BootstrapResult = &bootstrapResult

	log.Info("[BOOTSTRAP] complete")
	return result, nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(opts Options) Options {
	if opts.Mode == "" {
		opts.Mode = "modules"
	}
	if opts.Location == "" {
		opts.Location = "westeurope"
	}
	if opts.WorkDir == "" {
		opts.WorkDir = ".azlift"
	}
	if len(opts.Environments) == 0 {
		opts.Environments = []string{"prod", "staging", "dev"}
	}
	if opts.TargetSubscription == "" {
		opts.TargetSubscription = opts.SubscriptionID
	}
	return opts
}

// printDryRunPlan logs the actions that would be taken without executing them.
func printDryRunPlan(log *slog.Logger, opts Options) {
	log.Info("dry-run: no external tools will be called")
	log.Info(fmt.Sprintf("dry-run: [SCAN]    query subscription %s", opts.SubscriptionID))
	log.Info(fmt.Sprintf("dry-run: [EXPORT]  aztfexport resource-group %s → %s/raw/%s",
		opts.ResourceGroup, opts.WorkDir, opts.ResourceGroup))
	log.Info(fmt.Sprintf("dry-run: [REFINE]  %s/raw/%s → %s/refined (mode: %s)",
		opts.WorkDir, opts.ResourceGroup, opts.WorkDir, opts.Mode))
	if opts.Enrich || opts.FixSecurity {
		log.Info("dry-run: [REFINE]  enrichment pass enabled")
	}
	if opts.RepoName != "" && opts.RepoOrg != "" {
		isCross := opts.TargetTenant != "" && opts.TargetTenant != opts.TenantID
		log.Info(fmt.Sprintf("dry-run: [BOOTSTRAP] git init → gh repo create %s/%s → push",
			opts.RepoOrg, opts.RepoName))
		if isCross {
			log.Info("dry-run: [BOOTSTRAP] cross-tenant: bootstrap/ module will be generated")
		} else {
			log.Info(fmt.Sprintf("dry-run: [BOOTSTRAP] provision state storage + %d MI(s) + upload tfstate",
				len(opts.Environments)*2))
		}
	} else {
		log.Info("dry-run: [BOOTSTRAP] skipped (--repo-name / --org not set)")
	}
}
