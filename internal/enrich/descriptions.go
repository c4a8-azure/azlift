package enrich

import (
	"context"
	"fmt"
	"strings"

	"github.com/c4a8-azure/azlift/internal/refine"
)

const descriptionInstruction = `For every variable {} and output {} block that is missing a description attribute,
add a concise, human-readable description string.
Rules:
- Infer meaning from the block name and any default/type attributes.
- Keep descriptions under 80 characters.
- Do not change any existing attributes or blocks.
- Do not add descriptions to blocks that already have one.
Return ONLY the modified HCL with no prose.`

// EnrichDescriptions uses the AI client to add description fields to all
// variable and output blocks that lack one. Returns the enriched HCL.
//
// If client is nil, a no-op (passthrough) enrichment is performed — this
// allows the pipeline to run without --enrich without requiring an API key.
func EnrichDescriptions(ctx context.Context, client *Client, pf *refine.ParsedFile) (string, error) {
	content := string(pf.File.Bytes())

	// Skip files that have no variable or output blocks.
	if !strings.Contains(content, "variable") && !strings.Contains(content, "output") {
		return content, nil
	}

	if client == nil {
		return content, nil
	}

	resp, err := client.Enrich(ctx, EnrichRequest{
		Filename:    pf.Path,
		Content:     content,
		Instruction: descriptionInstruction,
	})
	if err != nil {
		return "", fmt.Errorf("enriching descriptions in %s: %w", pf.Path, err)
	}
	return resp.Content, nil
}

// EnrichDescriptionsAll runs EnrichDescriptions across all files that contain
// variable or output blocks, updating the in-memory AST. Returns a count of
// files that were enriched.
func EnrichDescriptionsAll(ctx context.Context, client *Client, files []*refine.ParsedFile) (int, error) {
	enriched := 0
	for _, pf := range files {
		content := string(pf.File.Bytes())
		if !strings.Contains(content, "variable") && !strings.Contains(content, "output") {
			continue
		}

		newContent, err := EnrichDescriptions(ctx, client, pf)
		if err != nil {
			return enriched, err
		}
		if newContent != content {
			// Re-parse the enriched content back into the ParsedFile's AST.
			if err := refine.ReplaceContent(pf, []byte(newContent)); err != nil {
				return enriched, fmt.Errorf("updating AST for %s: %w", pf.Path, err)
			}
			enriched++
		}
	}
	return enriched, nil
}
