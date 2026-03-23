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

// alwaysVariable are attribute names always extracted as input variables
// (user-facing, documented, overridable per environment).
var alwaysVariable = map[string]bool{
	"location": true,
}

// alwaysLocal are attribute names always extracted as locals
// (internal, derived, not expected to be overridden directly).
var alwaysLocal = map[string]bool{
	"resource_group_name": true,
}

// ExtractVariables scans all resource blocks across files for repeated string
// literals, extracts them into variables.tf and locals.tf, and rewrites the
// original references in-place. Returns the two generated files.
func ExtractVariables(files []*ParsedFile, outputDir string) (varsFile, localsFile *ParsedFile, err error) {
	// Count occurrences of each (attr → value) pair across all resource blocks.
	type attrVal struct{ attr, val string }
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

	// Decide which values to extract.
	type extraction struct {
		attr, val, ref string
		asVar          bool // true → variable block, false → local
	}
	var extractions []extraction
	seen := map[string]bool{} // deduplicate by attr name

	for av, count := range counts {
		if seen[av.attr] {
			continue
		}
		forceVar := alwaysVariable[av.attr]
		forceLocal := alwaysLocal[av.attr]
		if !forceVar && !forceLocal && count < extractThreshold {
			continue
		}
		seen[av.attr] = true

		localName := av.attr // use attr name directly as identifier
		ref := fmt.Sprintf("local.%s", localName)
		if forceVar {
			ref = fmt.Sprintf("var.%s", localName)
		}
		extractions = append(extractions, extraction{
			attr:  av.attr,
			val:   unquote(av.val),
			ref:   ref,
			asVar: forceVar,
		})
	}

	sort.Slice(extractions, func(i, j int) bool {
		return extractions[i].attr < extractions[j].attr
	})

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
			appendVariableBlock(varsFile.File.Body(), ex.attr, ex.val)
		} else {
			localsBlock.Body().SetAttributeValue(ex.attr, cty.StringVal(ex.val))
		}
	}

	// Rewrite matching attribute values in all resource blocks.
	refMap := map[string]string{}
	for _, ex := range extractions {
		refMap[ex.attr] = ex.ref
	}
	rewriteRefs(files, refMap)

	return varsFile, localsFile, nil
}

func appendVariableBlock(body *hclwrite.Body, name, defaultVal string) {
	block := body.AppendNewBlock("variable", []string{name})
	block.Body().SetAttributeRaw("description",
		hclwrite.TokensForValue(cty.StringVal(fmt.Sprintf("Azure %s for all resources.", strings.ReplaceAll(name, "_", " ")))))
	block.Body().SetAttributeRaw("type", hclwrite.TokensForIdentifier("string"))
	block.Body().SetAttributeValue("default", cty.StringVal(defaultVal))
	body.AppendNewline()
}

// rewriteRefs replaces matched attribute literals with variable/local refs.
func rewriteRefs(files []*ParsedFile, refMap map[string]string) {
	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			for attrName, ref := range refMap {
				attr := block.Body().GetAttribute(attrName)
				if attr == nil {
					continue
				}
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if !isStringLiteral(val) {
					continue
				}
				block.Body().SetAttributeRaw(attrName, hclwrite.TokensForIdentifier(ref))
			}
		}
	}
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
