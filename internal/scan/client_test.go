package scan

import (
	"context"
	"errors"
	"testing"
)

func TestMockClient_ReturnsRows(t *testing.T) {
	rows := []map[string]any{
		{"id": "/sub/rg-a/vm1", "resourceGroup": "rg-a", "type": "microsoft.compute/virtualmachines"},
	}
	mc := &MockClient{Rows: rows}

	got, err := mc.Query(context.Background(), []string{"sub-123"}, "Resources | limit 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	if got[0]["resourceGroup"] != "rg-a" {
		t.Errorf("resourceGroup: want rg-a, got %v", got[0]["resourceGroup"])
	}
}

func TestMockClient_PropagatesError(t *testing.T) {
	mc := &MockClient{Err: errors.New("auth failure")}
	_, err := mc.Query(context.Background(), []string{"sub-123"}, "Resources")
	if err == nil || err.Error() != "auth failure" {
		t.Errorf("expected auth failure error, got %v", err)
	}
}

func TestMockClient_RecordsCalls(t *testing.T) {
	mc := &MockClient{}
	_, _ = mc.Query(context.Background(), []string{"sub-a"}, "kql-1")
	_, _ = mc.Query(context.Background(), []string{"sub-b"}, "kql-2")

	if len(mc.Calls) != 2 {
		t.Fatalf("want 2 calls recorded, got %d", len(mc.Calls))
	}
	if mc.Calls[0].KQL != "kql-1" {
		t.Errorf("first call KQL: want kql-1, got %s", mc.Calls[0].KQL)
	}
	if mc.Calls[1].Subscriptions[0] != "sub-b" {
		t.Errorf("second call subscription: want sub-b, got %s", mc.Calls[1].Subscriptions[0])
	}
}

func TestExtractRows_NilData(t *testing.T) {
	rows, err := extractRows(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows != nil {
		t.Errorf("expected nil rows for nil input")
	}
}

func TestExtractRows_ValidSlice(t *testing.T) {
	data := []any{
		map[string]any{"id": "res-1"},
		map[string]any{"id": "res-2"},
	}
	rows, err := extractRows(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 rows, got %d", len(rows))
	}
}

func TestExtractRows_BadType(t *testing.T) {
	_, err := extractRows("not a slice")
	if err == nil {
		t.Error("expected error for non-slice input")
	}
}
