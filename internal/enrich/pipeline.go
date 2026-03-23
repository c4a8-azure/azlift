package enrich

import (
	"context"
	"fmt"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// Options controls the enrichment pipeline.
type Options struct {
	// APIKey is the Anthropic API key. When empty, AI-driven steps are skipped.
	APIKey string //nolint:gosec // key is read from env, not stored
	// Model overrides the default AI model.
	Model string
	// FixSecurity enables auto-remediation of security anti-patterns.
	FixSecurity bool
	// SkipLifecycle disables prevent_destroy injection.
	SkipLifecycle bool
	// SkipSecurity disables security anti-pattern scanning.
	SkipSecurity bool
	// SkipTags disables tag policy normalisation.
	SkipTags bool
	// SkipDescriptions disables AI description generation.
	SkipDescriptions bool
}

// RunResult summarises what the enrichment pipeline did.
type RunResult struct {
	// LifecycleInjected is the number of blocks that got prevent_destroy.
	LifecycleInjected int
	// SecurityFindings is the list of detected anti-patterns.
	SecurityFindings []SecurityFinding
	// TagsNormalised is the number of resource blocks whose tags were updated.
	TagsNormalised int
	// DescriptionsEnriched is the number of files enriched with AI descriptions.
	DescriptionsEnriched int
}

// Run executes the enrichment pipeline against the provided files.
// localsFile is the locals.tf ParsedFile used for tag normalisation.
func Run(ctx context.Context, files []*refine.ParsedFile, localsFile *refine.ParsedFile, opts Options) (RunResult, error) {
	var result RunResult

	// 1. Lifecycle inject (deterministic, no API).
	if !opts.SkipLifecycle {
		result.LifecycleInjected = InjectPreventDestroy(files, nil)
	}

	// 2. Security scan (deterministic, no API).
	if !opts.SkipSecurity {
		result.SecurityFindings = ScanSecurity(files, nil, opts.FixSecurity)
	}

	// 3. Tag normalisation (deterministic, no API).
	if !opts.SkipTags && localsFile != nil {
		result.TagsNormalised = NormaliseTags(files, localsFile)
	}

	// 4. AI description generation (requires API key).
	if !opts.SkipDescriptions && opts.APIKey != "" {
		client, err := buildClient(opts)
		if err != nil {
			return result, fmt.Errorf("initialising AI client: %w", err)
		}
		enriched, err := EnrichDescriptionsAll(ctx, client, files)
		if err != nil {
			return result, fmt.Errorf("AI description enrichment: %w", err)
		}
		result.DescriptionsEnriched = enriched
	}

	return result, nil
}

func buildClient(opts Options) (*Client, error) {
	if opts.Model != "" {
		return NewClientWithModel(opts.Model)
	}
	return NewClient()
}
