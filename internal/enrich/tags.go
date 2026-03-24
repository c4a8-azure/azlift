package enrich

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// StandardTagKeys are the tag keys that must appear in local.common_tags.
var StandardTagKeys = []string{
	"environment",
	"workload",
	"managed-by",
	"created-by",
	"cost-center",
}

// commonTagsLocalName is the local name used for the shared tags map.
const commonTagsLocalName = "common_tags"

// NormaliseTags performs two operations:
//
//  1. Injects a `common_tags` entry into the `locals {}` block of the
//     locals file, containing the standard tag keys with empty-string defaults
//     (the caller / environment will supply real values).
//  2. Rewrites every `tags = { ... }` attribute on resource blocks to use
//     `merge(local.common_tags, { <bespoke> })`, preserving any tags that
//     are not part of the standard set.
//
// Returns the count of resource blocks whose tags were normalised.
func NormaliseTags(files []*refine.ParsedFile, localsFile *refine.ParsedFile) int {
	// 1. Inject common_tags into locals.
	injectCommonTagsLocal(localsFile)

	// 2. Rewrite resource tags.
	normalised := 0
	for _, pf := range files {
		for _, block := range refine.Blocks(pf, "resource") {
			if normaliseTags(block) {
				normalised++
			}
		}
	}
	return normalised
}

// injectCommonTagsLocal adds (or overwrites) the common_tags attribute in
// the first locals {} block of localsFile.
func injectCommonTagsLocal(pf *refine.ParsedFile) {
	var localsBlock *hclwrite.Block
	for _, b := range pf.File.Body().Blocks() {
		if b.Type() == "locals" {
			localsBlock = b
			break
		}
	}
	if localsBlock == nil {
		localsBlock = pf.File.Body().AppendNewBlock("locals", nil)
	}

	// Build { key = "" ... } object for common_tags.
	src := buildCommonTagsObject()
	localsBlock.Body().SetAttributeRaw(commonTagsLocalName, hclwrite.TokensForIdentifier(src))
}

func buildCommonTagsObject() string {
	var sb strings.Builder
	sb.WriteString("{\n")
	keys := make([]string, len(StandardTagKeys))
	copy(keys, StandardTagKeys)
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "    %s = \"\"\n", k)
	}
	sb.WriteString("  }")
	return sb.String()
}

// normaliseTags rewrites the `tags` attribute of a resource block to use
// merge(local.common_tags, {...}). Returns true if the block was modified.
func normaliseTags(block *hclwrite.Block) bool {
	attr := block.Body().GetAttribute("tags")
	if attr == nil {
		// No tags attribute — inject a merge with empty bespoke map.
		block.Body().SetAttributeRaw("tags",
			hclwrite.TokensForIdentifier(
				fmt.Sprintf("merge(local.%s, {})", commonTagsLocalName)))
		return true
	}

	val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))

	// Already using merge(local.common_tags, ...) — idempotent.
	if strings.HasPrefix(val, fmt.Sprintf("merge(local.%s,", commonTagsLocalName)) ||
		strings.HasPrefix(val, fmt.Sprintf("merge(local.%s ,", commonTagsLocalName)) {
		return false
	}

	// Extract bespoke tags from the existing literal and wrap in merge().
	bespoke := extractBespokeTags(val)
	merged := fmt.Sprintf("merge(local.%s, %s)", commonTagsLocalName, bespoke)
	block.Body().SetAttributeRaw("tags", hclwrite.TokensForIdentifier(merged))
	return true
}

// extractBespokeTags filters out the standard tag keys from a literal object
// expression and returns the remainder as an HCL object literal. When there
// are no bespoke keys, returns "{}".
func extractBespokeTags(val string) string {
	// If val is not an object literal, wrap it directly (it may be a reference).
	if !strings.HasPrefix(val, "{") {
		return val
	}

	// Strip standard keys from the literal, keep the rest.
	standardSet := map[string]bool{}
	for _, k := range StandardTagKeys {
		standardSet[k] = true
	}

	inner := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(val), "}"), "{")
	var bespoke []string
	for _, line := range strings.Split(inner, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ",") && len(line) == 1 {
			continue
		}
		// Parse key = value (very naively — handles the common case).
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			bespoke = append(bespoke, line)
			continue
		}
		key := strings.Trim(strings.TrimSpace(line[:eqIdx]), `"`)
		if !standardSet[key] {
			bespoke = append(bespoke, strings.TrimSuffix(line, ","))
		}
	}

	if len(bespoke) == 0 {
		return "{}"
	}
	return "{\n    " + strings.Join(bespoke, "\n    ") + "\n  }"
}

// GenerateCommonTagsOutput appends an output block for common_tags to
// the outputs file so other modules can consume the merged tags.
func GenerateCommonTagsOutput(pf *refine.ParsedFile) {
	block := pf.File.Body().AppendNewBlock("output", []string{"common_tags"})
	block.Body().SetAttributeValue("description", cty.StringVal("Merged common tags applied to all resources."))
	block.Body().SetAttributeRaw("value",
		hclwrite.TokensForIdentifier(fmt.Sprintf("local.%s", commonTagsLocalName)))
}
