package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func sampleRows() []map[string]any {
	return []map[string]any{
		{"id": "/sub/rg-network/vnet1", "name": "vnet1", "type": "Microsoft.Network/virtualNetworks", "resourceGroup": "rg-network", "location": "westeurope", "subscriptionId": "sub-123"},
		{"id": "/sub/rg-network/nsg1", "name": "nsg1", "type": "Microsoft.Network/networkSecurityGroups", "resourceGroup": "rg-network", "location": "westeurope", "subscriptionId": "sub-123"},
		{"id": "/sub/rg-app/vm1", "name": "vm1", "type": "Microsoft.Compute/virtualMachines", "resourceGroup": "rg-app", "location": "westeurope", "subscriptionId": "sub-123"},
	}
}

func TestInventory_GroupsByResourceGroup(t *testing.T) {
	mc := &MockClient{Rows: sampleRows()}
	groups, err := Inventory(context.Background(), mc, "sub-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("want 2 resource groups, got %d", len(groups))
	}
	if groups["rg-network"].ResourceCount != 2 {
		t.Errorf("rg-network count: want 2, got %d", groups["rg-network"].ResourceCount)
	}
	if groups["rg-app"].ResourceCount != 1 {
		t.Errorf("rg-app count: want 1, got %d", groups["rg-app"].ResourceCount)
	}
}

func TestInventory_UniqueTypes(t *testing.T) {
	rows := []map[string]any{
		{"id": "1", "name": "a", "type": "Microsoft.Compute/virtualMachines", "resourceGroup": "rg-a", "location": "eastus", "subscriptionId": "sub-1"},
		{"id": "2", "name": "b", "type": "Microsoft.Compute/virtualMachines", "resourceGroup": "rg-a", "location": "eastus", "subscriptionId": "sub-1"},
	}
	mc := &MockClient{Rows: rows}
	groups, err := Inventory(context.Background(), mc, "sub-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups["rg-a"].ResourceTypes) != 1 {
		t.Errorf("want 1 unique type, got %d: %v", len(groups["rg-a"].ResourceTypes), groups["rg-a"].ResourceTypes)
	}
}

func TestInventory_EmptySubscription(t *testing.T) {
	mc := &MockClient{Rows: nil}
	groups, err := Inventory(context.Background(), mc, "sub-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("want 0 groups, got %d", len(groups))
	}
}

func TestPrintTable_ContainsHeaders(t *testing.T) {
	mc := &MockClient{Rows: sampleRows()}
	groups, _ := Inventory(context.Background(), mc, "sub-123")
	var buf bytes.Buffer
	PrintTable(&buf, groups)
	out := buf.String()
	if !strings.Contains(out, "Resource Group") {
		t.Error("table missing 'Resource Group' header")
	}
	if !strings.Contains(out, "rg-network") {
		t.Error("table missing rg-network row")
	}
	if !strings.Contains(out, "rg-app") {
		t.Error("table missing rg-app row")
	}
}

func TestPrintJSON_ValidJSON(t *testing.T) {
	mc := &MockClient{Rows: sampleRows()}
	groups, _ := Inventory(context.Background(), mc, "sub-123")
	var buf bytes.Buffer
	if err := PrintJSON(&buf, groups); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("want 2 groups in JSON, got %d", len(out))
	}
}

func TestTruncateList(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	got := truncateList(items, 3)
	if !strings.Contains(got, "(+2)") {
		t.Errorf("truncateList should show overflow count, got: %s", got)
	}
	short := truncateList(items[:2], 3)
	if strings.Contains(short, "(+") {
		t.Errorf("no truncation expected for short list, got: %s", short)
	}
}
