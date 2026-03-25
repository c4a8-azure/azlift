package enrich

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/c4a8-azure/azlift/internal/refine"
)

const (
	analysisMaxTokens = 8192

	// analysisBeginMarker / analysisEndMarker delimit the AI-generated section
	// inside README.md. Markers make injection idempotent: a second --enrich
	// run replaces the section rather than appending again.
	analysisBeginMarker = "<!-- BEGIN_AZLIFT_ANALYSIS -->"
	analysisEndMarker   = "<!-- END_AZLIFT_ANALYSIS -->"
)

const analysisSystemPrompt = `You are a senior Azure infrastructure architect reviewing Terraform code.
Produce a clear, structured architecture analysis document in Markdown.
Write for a DevOps/platform engineering audience who will use this as context when working with the infrastructure.`

const analysisUserPrompt = `Analyse the Terraform code below and produce a Markdown architecture analysis with exactly these sections:

## Overview
A 2–4 sentence summary of what infrastructure this module manages and its purpose.

## Resource Inventory
A concise grouped list of key resources (e.g. Networking, Identity, Compute, Storage). Skip purely boilerplate resources.

## Architecture & Relationships
Topology and key dependencies between resources. Highlight cross-resource-group or external references.

## Security Posture
Security-relevant configuration: private endpoints, network restrictions, identity/RBAC, Key Vault integration. Note any gaps.

## Operational Notes
Noteworthy lifecycle rules, tags, naming conventions, or configuration patterns operators should know about.

Return ONLY the Markdown — no prose outside the sections, no surrounding code fence.

---

`

// GenerateAnalysis sends all .tf files to the AI and returns a Markdown
// architecture analysis. Returns ("", nil) when client is nil or no content.
func GenerateAnalysis(ctx context.Context, client *Client, files []*refine.ParsedFile, log *slog.Logger) (string, error) {
	if client == nil {
		return "", nil
	}
	if log == nil {
		log = slog.Default()
	}

	// Concatenate all files into a single context block, labelled by filename.
	var sb strings.Builder
	for _, pf := range files {
		content := strings.TrimSpace(string(pf.File.Bytes()))
		if content == "" {
			continue
		}
		fmt.Fprintf(&sb, "### %s\n\n```hcl\n%s\n```\n\n", filepath.Base(pf.Path), content)
	}
	combined := sb.String()
	if combined == "" {
		return "", nil
	}

	log.Debug("enrich: sending all TF files to AI for architecture analysis")

	msg, err := client.api.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(client.model),
		MaxTokens: analysisMaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: analysisSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(analysisUserPrompt + combined)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic API (analysis): %w", err)
	}
	if len(msg.Content) == 0 {
		return "", nil
	}
	return msg.Content[0].Text, nil
}

// WriteAnalysis writes the analysis markdown to ANALYSIS.md in the same
// directory as the first file, then injects it into README.md using
// BEGIN/END markers so repeated runs replace rather than append.
// Returns the path to ANALYSIS.md, or "" when content is empty.
func WriteAnalysis(outputDir, content string) (string, error) {
	if content == "" {
		return "", nil
	}

	// Write standalone ANALYSIS.md.
	analysisPath := filepath.Join(outputDir, "ANALYSIS.md")
	if err := os.WriteFile(analysisPath, []byte(content+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing ANALYSIS.md: %w", err)
	}

	// Inject into README.md when it exists.
	readmePath := filepath.Join(outputDir, "README.md")
	readmeBytes, err := os.ReadFile(readmePath) //nolint:gosec
	if os.IsNotExist(err) {
		return analysisPath, nil
	}
	if err != nil {
		return analysisPath, fmt.Errorf("reading README.md: %w", err)
	}

	section := analysisBeginMarker + "\n\n" + content + "\n\n" + analysisEndMarker
	readme := string(readmeBytes)

	var updated string
	begin := strings.Index(readme, analysisBeginMarker)
	end := strings.Index(readme, analysisEndMarker)
	if begin != -1 && end != -1 && end > begin {
		// Replace existing section.
		updated = readme[:begin] + section + readme[end+len(analysisEndMarker):]
	} else {
		// Append after existing content.
		updated = strings.TrimRight(readme, "\n") + "\n\n---\n\n" + section + "\n"
	}

	if err := os.WriteFile(readmePath, []byte(updated), 0o600); err != nil {
		return analysisPath, fmt.Errorf("injecting analysis into README.md: %w", err)
	}
	return analysisPath, nil
}
