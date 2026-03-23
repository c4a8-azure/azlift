package scan

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

const (
	defaultPageSize = 1000
)

// Client wraps the Azure Resource Graph API with pagination and a mockable
// interface so callers can substitute a fake in unit tests.
type Client interface {
	// Query executes a KQL query against the given subscriptions and returns
	// all matching rows, handling pagination transparently.
	Query(ctx context.Context, subscriptions []string, kql string) ([]map[string]any, error)
}

// NewClient creates a production Client backed by DefaultAzureCredential.
// The credential chain is: Azure CLI → Workload Identity → Managed Identity
// → environment variables — matching the standard az-login flow.
func NewClient() (Client, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("building Azure credential: %w", err)
	}
	inner, err := armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("building Resource Graph client: %w", err)
	}
	return &resourceGraphClient{inner: inner}, nil
}

// resourceGraphClient is the production implementation of Client.
type resourceGraphClient struct {
	inner *armresourcegraph.Client
}

// Query runs kql against the provided subscriptions, collecting all pages.
func (c *resourceGraphClient) Query(ctx context.Context, subscriptions []string, kql string) ([]map[string]any, error) {
	var all []map[string]any
	var skipToken *string

	pageSize := int32(defaultPageSize)

	for {
		req := armresourcegraph.QueryRequest{
			Subscriptions: toStringPtrs(subscriptions),
			Query:         &kql,
			Options: &armresourcegraph.QueryRequestOptions{
				ResultFormat: resultFormatPtr(armresourcegraph.ResultFormatObjectArray),
				Top:          &pageSize,
				SkipToken:    skipToken,
			},
		}

		resp, err := c.inner.Resources(ctx, req, nil)
		if err != nil {
			return nil, fmt.Errorf("resource graph query: %w", err)
		}

		rows, err := extractRows(resp.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)

		if resp.SkipToken == nil || *resp.SkipToken == "" {
			break
		}
		skipToken = resp.SkipToken
	}

	return all, nil
}

// extractRows casts the untyped response data to the expected row format.
func extractRows(data any) ([]map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	slice, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected resource graph response type %T", data)
	}
	rows := make([]map[string]any, 0, len(slice))
	for _, item := range slice {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected row type %T", item)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func toStringPtrs(ss []string) []*string {
	out := make([]*string, len(ss))
	for i := range ss {
		s := ss[i]
		out[i] = &s
	}
	return out
}

func resultFormatPtr(f armresourcegraph.ResultFormat) *armresourcegraph.ResultFormat {
	return &f
}
