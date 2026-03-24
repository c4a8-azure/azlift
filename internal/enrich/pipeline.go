package enrich

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go/option"

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
	// Log is an optional structured logger for progress output.
	Log *slog.Logger
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
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	// 1. Lifecycle inject (deterministic, no API).
	if !opts.SkipLifecycle {
		log.Debug("enrich: injecting lifecycle prevent_destroy")
		result.LifecycleInjected = InjectPreventDestroy(files, nil)
		log.Info(fmt.Sprintf("enrich: lifecycle — %d block(s) protected with prevent_destroy", result.LifecycleInjected))
	}

	// 2. Security scan (deterministic, no API).
	if !opts.SkipSecurity {
		log.Debug("enrich: scanning for security anti-patterns")
		result.SecurityFindings = ScanSecurity(files, nil, opts.FixSecurity)
		if len(result.SecurityFindings) == 0 {
			log.Info("enrich: security — no issues found")
		} else {
			for _, f := range result.SecurityFindings {
				status := "found"
				if f.Fixed {
					status = "fixed"
				}
				log.Warn(fmt.Sprintf("enrich: security [%s] %s.%s — %s (%s)",
					status, f.ResourceType, f.ResourceName, f.Message, f.Attribute))
			}
		}
	}

	// 3. Tag normalisation (deterministic, no API).
	if !opts.SkipTags && localsFile != nil {
		log.Debug("enrich: normalising tag policy")
		result.TagsNormalised = NormaliseTags(files, localsFile)
		log.Info(fmt.Sprintf("enrich: tags — %d resource(s) normalised to merge(local.common_tags, {...})", result.TagsNormalised))
	}

	// 4. AI description generation (requires API key).
	if !opts.SkipDescriptions {
		if opts.APIKey == "" {
			log.Debug("enrich: skipping AI descriptions — ANTHROPIC_API_KEY not set")
		} else {
			log.Info(fmt.Sprintf("enrich: sending files to AI model (%s) for description generation", modelName(opts)))
			client, err := buildClient(opts)
			if err != nil {
				return result, fmt.Errorf("initialising AI client: %w", err)
			}
			enriched, err := EnrichDescriptionsAll(ctx, client, files, log)
			if err != nil {
				return result, fmt.Errorf("AI description enrichment: %w", err)
			}
			result.DescriptionsEnriched = enriched
			log.Info(fmt.Sprintf("enrich: AI descriptions — %d file(s) enriched", enriched))
		}
	}

	return result, nil
}

// buildClient constructs an Anthropic client using opts.APIKey directly,
// falling back to the ANTHROPIC_API_KEY environment variable when empty.
func buildClient(opts Options) (*Client, error) {
	// Use the key from opts when explicitly provided (e.g. from env var read
	// at CLI startup), so it is not read a second time from the environment.
	clientOpts := []option.RequestOption{}
	if opts.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(opts.APIKey))
	}

	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	c, err := newClientWithOptions(model, clientOpts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func modelName(opts Options) string {
	if opts.Model != "" {
		return opts.Model
	}
	return string(DefaultModel)
}
