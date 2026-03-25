package refine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

const extractThreshold = 3 // extract literals appearing in 3+ resource blocks

// attrVal is a (attribute-name, quoted-value) pair used as a map key.
type attrVal struct{ attr, val string }

// alwaysVariable are attribute names always extracted as input variables
// (user-facing, overridable per environment). Each distinct value gets its
// own numbered variable when multiple values are present (e.g.
// resource_group_name_001, resource_group_name_002).
var alwaysVariable = map[string]bool{
	"location":            true,
	"resource_group_name": true,
}

// alwaysLocal are attribute names always extracted as locals
// (internal, not expected to be overridden directly).
var alwaysLocal = map[string]bool{}

// ExtractVariables scans all resource blocks across files for repeated string
// literals, extracts them into variables.tf and locals.tf, and rewrites the
// original references in-place. Returns the two generated files.
//
// When multiple distinct values qualify for the same attribute name (e.g.
// virtual_network_name = "vnet-a" and virtual_network_name = "vnet-b"), each
// value gets its own numbered local (virtual_network_name_001, _002, …) sorted
// alphabetically for determinism. This prevents the previous bug where a single
// value was chosen arbitrarily and rewritten onto all occurrences.
func ExtractVariables(files []*ParsedFile, outputDir string) (varsFile, localsFile *ParsedFile, err error) {
	// Count occurrences of each (attr → value) pair across all resource blocks.
	counts := map[attrVal]int{}

	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			for name, attr := range block.Body().Attributes() {
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				// Only count plain string literals (quoted, no interpolation).
				if isStringLiteral(val) {
					counts[attrVal{name, val}]++
				}
			}
		}
	}

	// Group qualifying (attr, val) pairs by attr name.
	// A pair qualifies when it is in alwaysVariable/alwaysLocal OR appears
	// in at least extractThreshold resource blocks.
	type attrGroup struct {
		vals map[string]int // quoted-val → count
	}
	groups := map[string]*attrGroup{}

	for av, count := range counts {
		forceVar := alwaysVariable[av.attr]
		forceLocal := alwaysLocal[av.attr]
		if !forceVar && !forceLocal && count < extractThreshold {
			continue
		}
		if groups[av.attr] == nil {
			groups[av.attr] = &attrGroup{vals: map[string]int{}}
		}
		groups[av.attr].vals[av.val] = count
	}

	// Build extractions sorted by attr name for a stable output order.
	// Single value → use plain attr name (local.resource_group_name).
	// Multiple values → append a 1-based zero-padded index suffix sorted
	// alphabetically by value (local.virtual_network_name_001, _002, …).
	type extraction struct {
		attr      string // original attribute name
		localName string // identifier used in locals.tf / variables.tf
		val       string // unquoted value
		ref       string // expression written back into resource blocks
		asVar     bool   // true → variable block, false → local
	}
	var extractions []extraction

	attrNames := make([]string, 0, len(groups))
	for attr := range groups {
		attrNames = append(attrNames, attr)
	}
	sort.Strings(attrNames)

	for _, attr := range attrNames {
		ag := groups[attr]
		sortedVals := sortedStringKeys(ag.vals) // alphabetical, deterministic

		for i, quotedVal := range sortedVals {
			localName := attr
			if len(sortedVals) > 1 {
				localName = fmt.Sprintf("%s_%03d", attr, i+1)
			}
			ref := fmt.Sprintf("local.%s", localName)
			if alwaysVariable[attr] {
				ref = fmt.Sprintf("var.%s", localName)
			}
			extractions = append(extractions, extraction{
				attr:      attr,
				localName: localName,
				val:       unquote(quotedVal),
				ref:       ref,
				asVar:     alwaysVariable[attr],
			})
		}
	}

	if len(extractions) == 0 {
		return NewFile(filepath.Join(outputDir, "variables.tf")),
			NewFile(filepath.Join(outputDir, "locals.tf")), nil
	}

	// Build variables.tf and locals.tf.
	varsFile = NewFile(filepath.Join(outputDir, "variables.tf"))
	localsFile = NewFile(filepath.Join(outputDir, "locals.tf"))
	localsBlock := localsFile.File.Body().AppendNewBlock("locals", nil)

	for _, ex := range extractions {
		if ex.asVar {
			appendVariableBlock(varsFile.File.Body(), ex.localName, ex.val)
		} else {
			localsBlock.Body().SetAttributeValue(ex.localName, cty.StringVal(ex.val))
		}
	}

	// Rewrite matching (attr, value) pairs in all resource blocks.
	// The refMap is keyed by (attr, quoted-val) so each value is rewritten to
	// its own ref — different values for the same attr stay independent.
	refMap := map[attrVal]string{}
	for _, ex := range extractions {
		refMap[attrVal{ex.attr, `"` + ex.val + `"`}] = ex.ref
	}
	rewriteRefs(files, refMap)

	return varsFile, localsFile, nil
}

func appendVariableBlock(body *hclwrite.Body, name, defaultVal string) {
	block := body.AppendNewBlock("variable", []string{name})
	block.Body().SetAttributeRaw("type", hclwrite.TokensForIdentifier("string"))
	block.Body().SetAttributeValue("default", cty.StringVal(defaultVal))
	body.AppendNewline()
}

// rewriteRefs replaces matched (attr, value) pairs with variable/local refs.
// Only attributes whose value exactly matches the extracted value are rewritten;
// other occurrences of the same attribute name with different values are left
// untouched, preventing cross-resource contamination.
func rewriteRefs(files []*ParsedFile, refMap map[attrVal]string) {
	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			for key, ref := range refMap {
				attr := block.Body().GetAttribute(key.attr)
				if attr == nil {
					continue
				}
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if val != key.val {
					continue // value doesn't match — leave untouched
				}
				block.Body().SetAttributeRaw(key.attr, hclwrite.TokensForIdentifier(ref))
			}
		}
	}
}

// sortedStringKeys returns the keys of m sorted alphabetically.
func sortedStringKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isStringLiteral returns true for simple quoted strings with no interpolation.
func isStringLiteral(expr string) bool {
	return strings.HasPrefix(expr, `"`) &&
		strings.HasSuffix(expr, `"`) &&
		!strings.Contains(expr, "${")
}

// unquote strips surrounding double-quotes from a string literal token.
func unquote(s string) string {
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	return s
}
