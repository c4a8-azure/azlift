package refine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// StandardTagKeys are the tag keys that must appear in local.common_tags.
var StandardTagKeys = []string{
	"environment",
	"workload",
	"managed-by",
	"created-by",
	"cost-center",
}

// CommonTagsLocalName is the local name used for the shared tags map.
const CommonTagsLocalName = "common_tags"

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
func NormaliseTags(files []*ParsedFile, localsFile *ParsedFile) int {
	injectCommonTagsLocal(localsFile)

	normalised := 0
	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			if normaliseTagsBlock(block) {
				normalised++
			}
		}
	}
	return normalised
}

// injectCommonTagsLocal adds (or overwrites) the common_tags attribute in
// the first locals {} block of localsFile.
func injectCommonTagsLocal(pf *ParsedFile) {
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

	localsBlock.Body().SetAttributeRaw(CommonTagsLocalName, hclwrite.TokensForIdentifier(buildCommonTagsObject()))
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

// normaliseTagsBlock rewrites the `tags` attribute of a resource block to use
// merge(local.common_tags, {...}). Returns true if the block was modified.
func normaliseTagsBlock(block *hclwrite.Block) bool {
	attr := block.Body().GetAttribute("tags")
	if attr == nil {
		block.Body().SetAttributeRaw("tags",
			hclwrite.TokensForIdentifier(fmt.Sprintf("merge(local.%s, {})", CommonTagsLocalName)))
		return true
	}

	val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))

	// Already normalised — idempotent.
	if strings.HasPrefix(val, fmt.Sprintf("merge(local.%s,", CommonTagsLocalName)) ||
		strings.HasPrefix(val, fmt.Sprintf("merge(local.%s ,", CommonTagsLocalName)) {
		return false
	}

	bespoke := extractBespokeTags(val)
	block.Body().SetAttributeRaw("tags",
		hclwrite.TokensForIdentifier(fmt.Sprintf("merge(local.%s, %s)", CommonTagsLocalName, bespoke)))
	return true
}

// extractBespokeTags filters out the standard tag keys from a literal object
// expression and returns the remainder as an HCL object literal.
// When there are no bespoke keys, returns "{}".
func extractBespokeTags(val string) string {
	if !strings.HasPrefix(val, "{") {
		return val
	}

	standardSet := map[string]bool{}
	for _, k := range StandardTagKeys {
		standardSet[k] = true
	}

	inner := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(val), "}"), "{")
	var bespoke []string
	for _, line := range strings.Split(inner, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || (strings.HasSuffix(line, ",") && len(line) == 1) {
			continue
		}
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
