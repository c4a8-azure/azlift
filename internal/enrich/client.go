// Package enrich implements the AI enrichment pass (--enrich flag) for the
// azlift refine stage. Each .tf file is sent independently to the Anthropic
// API; enrichment is deterministic given the same model and prompt.
package enrich

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	// DefaultModel is the Anthropic model used for enrichment.
	// Haiku is fast and cheap for deterministic HCL transformations.
	DefaultModel = anthropic.ModelClaudeHaiku4_5

	// maxTokens is the upper bound for enrichment responses.
	maxTokens = 4096
)

// Client wraps the Anthropic API for HCL enrichment operations.
type Client struct {
	api   anthropic.MessageService
	model string
}

// NewClient constructs a Client from ANTHROPIC_API_KEY.
// Returns a clear error when the key is missing.
func NewClient() (*Client, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New(
			"ANTHROPIC_API_KEY environment variable is not set; " +
				"set it to your Anthropic API key before using --enrich")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	return &Client{api: c.Messages, model: DefaultModel}, nil
}

// NewClientWithModel constructs a Client with an explicit model name.
func NewClientWithModel(model string) (*Client, error) {
	c, err := NewClient()
	if err != nil {
		return nil, err
	}
	c.model = model
	return c, nil
}

// EnrichRequest carries a single HCL file for enrichment.
type EnrichRequest struct {
	// Filename is the base name of the .tf file (used in the prompt).
	Filename string
	// Content is the current HCL source text.
	Content string
	// Instruction is the specific enrichment task (e.g. "add descriptions").
	Instruction string
}

// EnrichResponse holds the enriched HCL returned by the model.
type EnrichResponse struct {
	// Content is the enriched HCL source text.
	Content string
	// InputTokens is the number of tokens in the request.
	InputTokens int64
	// OutputTokens is the number of tokens in the response.
	OutputTokens int64
}

// systemPrompt instructs the model to act as a Terraform expert.
const systemPrompt = `You are an expert Terraform engineer specialising in Azure infrastructure.
You will be given a Terraform HCL file and a specific enrichment instruction.
Respond ONLY with the enriched HCL — no prose, no markdown fences, no explanation.
The output must be valid Terraform HCL that can be parsed by terraform init.
Do not remove any existing blocks or attributes.
Enrichment must be idempotent: running the same enrichment twice produces the same result.`

// Enrich sends a single HCL file to the API for enrichment and returns
// the modified content. The caller is responsible for writing the result
// back to disk.
func (c *Client) Enrich(ctx context.Context, req EnrichRequest) (EnrichResponse, error) {
	userMsg := fmt.Sprintf(
		"File: %s\n\nInstruction: %s\n\nHCL:\n%s",
		req.Filename, req.Instruction, req.Content,
	)

	msg, err := c.api.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg)),
		},
	})
	if err != nil {
		return EnrichResponse{}, fmt.Errorf("anthropic API: %w", err)
	}

	if len(msg.Content) == 0 {
		return EnrichResponse{}, errors.New("anthropic API returned empty content")
	}

	text := msg.Content[0].Text
	return EnrichResponse{
		Content:      text,
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
	}, nil
}
