package scan

import "context"

// MockClient is a test double for Client. Tests set Rows to control what
// Query returns, and can inspect Calls to verify query behaviour.
type MockClient struct {
	// Rows is returned on every Query call (shared across calls).
	Rows []map[string]any
	// Err is returned instead of Rows when non-nil.
	Err error
	// Calls records every (subscriptions, kql) pair passed to Query.
	Calls []MockCall
}

// MockCall captures one Query invocation for assertion in tests.
type MockCall struct {
	Subscriptions []string
	KQL           string
}

// Query implements Client.
func (m *MockClient) Query(_ context.Context, subscriptions []string, kql string) ([]map[string]any, error) {
	m.Calls = append(m.Calls, MockCall{Subscriptions: subscriptions, KQL: kql})
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Rows, nil
}
