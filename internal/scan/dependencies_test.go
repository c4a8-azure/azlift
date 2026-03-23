package scan

import (
	"testing"
)

func makeGroups(resources map[string][]ResourceSummary) map[string]*ResourceGroup {
	groups := map[string]*ResourceGroup{}
	for rg, res := range resources {
		groups[rg] = &ResourceGroup{Name: rg, Resources: res, ResourceCount: len(res)}
	}
	return groups
}

func TestAnalyseDependencies_NICRefersToVNet(t *testing.T) {
	groups := makeGroups(map[string][]ResourceSummary{
		"rg-app": {
			{
				ID:            "/subscriptions/sub/resourceGroups/rg-app/providers/Microsoft.Network/networkInterfaces/nic1",
				Name:          "nic1",
				Type:          "microsoft.network/networkinterfaces",
				ResourceGroup: "rg-app",
				Properties: map[string]any{
					"ipConfigurations": []any{
						map[string]any{
							"properties": map[string]any{
								"subnet": map[string]any{
									"id": "/subscriptions/sub/resourceGroups/rg-network/providers/Microsoft.Network/virtualNetworks/vnet1/subnets/default",
								},
							},
						},
					},
				},
			},
		},
		"rg-network": {
			{ID: "/subscriptions/sub/resourceGroups/rg-network/providers/Microsoft.Network/virtualNetworks/vnet1", Name: "vnet1", Type: "microsoft.network/virtualnetworks", ResourceGroup: "rg-network"},
		},
	})

	graph := AnalyseDependencies(groups)

	if len(graph.Edges) != 1 {
		t.Fatalf("want 1 edge, got %d: %+v", len(graph.Edges), graph.Edges)
	}
	if graph.Edges[0].From != "rg-app" || graph.Edges[0].To != "rg-network" {
		t.Errorf("wrong edge: %+v", graph.Edges[0])
	}
	if graph.Edges[0].Reason != "VNet/Subnet reference" {
		t.Errorf("wrong reason: %s", graph.Edges[0].Reason)
	}
}

func TestAnalyseDependencies_SameRGReferenceIgnored(t *testing.T) {
	groups := makeGroups(map[string][]ResourceSummary{
		"rg-a": {
			{
				ID:            "/subscriptions/sub/resourceGroups/rg-a/providers/Microsoft.Network/networkInterfaces/nic1",
				Type:          "microsoft.network/networkinterfaces",
				ResourceGroup: "rg-a",
				Properties: map[string]any{
					"ipConfigurations": []any{
						map[string]any{
							"properties": map[string]any{
								"subnet": map[string]any{
									"id": "/subscriptions/sub/resourceGroups/rg-a/providers/Microsoft.Network/virtualNetworks/vnet1/subnets/default",
								},
							},
						},
					},
				},
			},
			{ID: "/subscriptions/sub/resourceGroups/rg-a/providers/Microsoft.Network/virtualNetworks/vnet1", Type: "microsoft.network/virtualnetworks", ResourceGroup: "rg-a"},
		},
	})

	graph := AnalyseDependencies(groups)
	if len(graph.Edges) != 0 {
		t.Errorf("intra-RG reference should be ignored, got edges: %+v", graph.Edges)
	}
}

func TestAnalyseDependencies_DeduplicatesEdges(t *testing.T) {
	subnet := map[string]any{
		"id": "/subscriptions/sub/resourceGroups/rg-network/providers/Microsoft.Network/virtualNetworks/vnet1/subnets/default",
	}
	props := map[string]any{
		"ipConfigurations": []any{
			map[string]any{"properties": map[string]any{"subnet": subnet}},
			map[string]any{"properties": map[string]any{"subnet": subnet}}, // duplicate
		},
	}
	groups := makeGroups(map[string][]ResourceSummary{
		"rg-app": {
			{ID: "nic1", Type: "microsoft.network/networkinterfaces", ResourceGroup: "rg-app", Properties: props},
		},
		"rg-network": {
			{ID: "vnet1", Type: "microsoft.network/virtualnetworks", ResourceGroup: "rg-network"},
		},
	})

	graph := AnalyseDependencies(groups)
	if len(graph.Edges) != 1 {
		t.Errorf("duplicate edges should be deduplicated, got %d", len(graph.Edges))
	}
}

func TestAnalyseDependencies_NoProperties(t *testing.T) {
	groups := makeGroups(map[string][]ResourceSummary{
		"rg-a": {{ID: "r1", Type: "microsoft.compute/virtualmachines", ResourceGroup: "rg-a", Properties: nil}},
	})
	graph := AnalyseDependencies(groups)
	if len(graph.Edges) != 0 {
		t.Errorf("resource with no properties should produce no edges")
	}
}

// TestAnalyseDependencies_VMNilDiskEncryptionSet reproduces the panic that
// occurred with real Azure data: managedDisk present but diskEncryptionSet nil.
func TestAnalyseDependencies_VMNilDiskEncryptionSet(t *testing.T) {
	groups := makeGroups(map[string][]ResourceSummary{
		"rg-app": {
			{
				ID:            "/subscriptions/sub/resourceGroups/rg-app/providers/Microsoft.Compute/virtualMachines/vm1",
				Type:          "microsoft.compute/virtualmachines",
				ResourceGroup: "rg-app",
				Properties: map[string]any{
					"storageProfile": map[string]any{
						"osDisk": map[string]any{
							"managedDisk": map[string]any{
								"diskEncryptionSet": nil, // real Azure returns nil when not set
							},
						},
					},
				},
			},
		},
	})
	// Must not panic.
	graph := AnalyseDependencies(groups)
	if len(graph.Edges) != 0 {
		t.Errorf("expected no edges for VM without cross-RG refs, got %d", len(graph.Edges))
	}
}

func TestResourceGroupFromID(t *testing.T) {
	cases := []struct{ id, want string }{
		{"/subscriptions/sub/resourceGroups/rg-prod/providers/Microsoft.Compute/virtualMachines/vm1", "rg-prod"},
		{"/subscriptions/sub/resourceGroups/MY-RG/providers/foo/bar", "MY-RG"},
		{"not-an-arm-id", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := resourceGroupFromID(c.id)
		if got != c.want {
			t.Errorf("resourceGroupFromID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestDependsOn(t *testing.T) {
	g := &DependencyGraph{Edges: []Dependency{
		{From: "rg-app", To: "rg-network", Reason: "VNet/Subnet reference"},
		{From: "rg-app", To: "rg-data", Reason: "Storage VNet rule"},
	}}
	deps := g.DependsOn("rg-app")
	if len(deps) != 2 {
		t.Errorf("want 2 dependencies, got %d: %v", len(deps), deps)
	}
	if len(g.DependsOn("rg-network")) != 0 {
		t.Error("rg-network should have no dependencies")
	}
}
