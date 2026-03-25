package enrich

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// GenerateCommonTagsOutput appends an output block for common_tags to
// the outputs file so other modules can consume the merged tags.
func GenerateCommonTagsOutput(pf *refine.ParsedFile) {
	block := pf.File.Body().AppendNewBlock("output", []string{"common_tags"})
	block.Body().SetAttributeValue("description", cty.StringVal("Merged common tags applied to all resources."))
	block.Body().SetAttributeRaw("value",
		hclwrite.TokensForIdentifier(fmt.Sprintf("local.%s", refine.CommonTagsLocalName)))
}
