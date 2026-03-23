package scan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Dependency records a directed reference from one resource group to another.
type Dependency struct {
	// From is the resource group that contains the referencing resource.
	From string `json:"from"`
	// To is the resource group that owns the referenced resource.
	To string `json:"to"`
	// Reason describes what kind of reference was detected.
	Reason string `json:"reason"`
	// SourceResourceID is the resource that holds the reference.
	SourceResourceID string `json:"sourceResourceId"`
}

// DependencyGraph maps each resource group to the set of other resource
// groups it depends on, together with the reasons.
type DependencyGraph struct {
	// Edges is the complete list of detected cross-RG references.
	Edges []Dependency `json:"edges"`
}

// DependsOn returns all resource groups that rgName depends on.
func (g *DependencyGraph) DependsOn(rgName string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range g.Edges {
		if e.From == rgName {
			if _, ok := seen[e.To]; !ok {
				seen[e.To] = struct{}{}
				out = append(out, e.To)
			}
		}
	}
	return out
}

// AnalyseDependencies inspects resource properties to find cross-RG
// references and returns a DependencyGraph. The analysis is best-effort:
// it parses known property patterns for VNet IDs, Key Vault references,
// and Storage Account references. Unknown resource types are skipped.
func AnalyseDependencies(groups map[string]*ResourceGroup) *DependencyGraph {
	graph := &DependencyGraph{}

	for rgName, rg := range groups {
		for _, res := range rg.Resources {
			refs := extractReferences(res)
			for _, ref := range refs {
				targetRG := strings.ToLower(resourceGroupFromID(ref.id))
				if targetRG == "" || targetRG == rgName {
					continue
				}
				if _, ok := groups[targetRG]; !ok {
					continue // reference points outside the scanned scope
				}
				graph.Edges = append(graph.Edges, Dependency{
					From:             rgName,
					To:               targetRG,
					Reason:           ref.reason,
					SourceResourceID: res.ID,
				})
			}
		}
	}

	deduplicateEdges(graph)
	return graph
}

// reference is an internal helper holding one extracted ID and its label.
type reference struct {
	id     string
	reason string
}

// extractReferences walks known property paths for a resource and returns
// any Azure resource IDs that point outside the resource's own RG.
func extractReferences(res ResourceSummary) []reference {
	if res.Properties == nil {
		return nil
	}

	var refs []reference

	switch {
	case strings.HasSuffix(res.Type, "networkinterfaces"):
		// NIC → VNet/Subnet
		refs = append(refs, extractIPConfigRefs(res.Properties)...)

	case strings.HasSuffix(res.Type, "virtualmachines"):
		// VM → Key Vault (disk encryption), storage diagnostics
		refs = append(refs, extractVMRefs(res.Properties)...)

	case strings.HasSuffix(res.Type, "storageaccounts"):
		// Storage may reference a VNet for service endpoint rules
		refs = append(refs, extractStorageRefs(res.Properties)...)
	}

	return refs
}

func extractIPConfigRefs(props map[string]any) []reference {
	var refs []reference
	ipConfigs := nestedSlice(props, "ipConfigurations")
	for _, cfg := range ipConfigs {
		m, _ := cfg.(map[string]any)
		if m == nil {
			continue
		}
		inner, _ := m["properties"].(map[string]any)
		if inner == nil {
			continue
		}
		if subnet, ok := inner["subnet"].(map[string]any); ok {
			if id, _ := subnet["id"].(string); id != "" {
				refs = append(refs, reference{id: id, reason: "VNet/Subnet reference"})
			}
		}
	}
	return refs
}

func extractVMRefs(props map[string]any) []reference {
	var refs []reference
	// Disk encryption set reference
	if sp, ok := props["storageProfile"].(map[string]any); ok {
		if os, ok := sp["osDisk"].(map[string]any); ok {
			if enc, ok := os["managedDisk"].(map[string]any); ok {
				if des, ok := enc["diskEncryptionSet"].(map[string]any); ok {
					if id, _ := des["id"].(string); id != "" {
						refs = append(refs, reference{id: id, reason: "Disk Encryption Set reference"})
					}
				}
			}
		}
	}
	// Diagnostics storage account
	if diag, ok := props["diagnosticsProfile"].(map[string]any); ok {
		if boot, ok := diag["bootDiagnostics"].(map[string]any); ok {
			if uri, _ := boot["storageUri"].(string); uri != "" {
				// storageUri is a URL, not an ARM ID — extract account name
				// and skip (cannot resolve to RG without additional lookup)
				_ = uri
			}
		}
	}
	return refs
}

func extractStorageRefs(props map[string]any) []reference {
	var refs []reference
	// Virtual network rules on storage accounts
	if netAcl, ok := props["networkAcls"].(map[string]any); ok {
		rules, _ := netAcl["virtualNetworkRules"].([]any)
		for _, rule := range rules {
			m, _ := rule.(map[string]any)
			if id, _ := m["id"].(string); id != "" {
				refs = append(refs, reference{id: id, reason: "Storage VNet rule"})
			}
		}
	}
	return refs
}

// resourceGroupFromID extracts the resource group name from an Azure ARM ID.
// ARM IDs have the form: /subscriptions/<sub>/resourceGroups/<rg>/...
func resourceGroupFromID(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// deduplicateEdges removes edges that have identical From/To/Reason tuples.
func deduplicateEdges(g *DependencyGraph) {
	seen := map[string]struct{}{}
	unique := g.Edges[:0]
	for _, e := range g.Edges {
		key := fmt.Sprintf("%s→%s:%s", e.From, e.To, e.Reason)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			unique = append(unique, e)
		}
	}
	g.Edges = unique
}

// nestedSlice safely navigates a properties map and returns a []any at key.
func nestedSlice(m map[string]any, key string) []any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, _ := v.([]any)
	return s
}

// MarshalJSON serialises the dependency graph as indented JSON.
func (g *DependencyGraph) MarshalPretty() ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}
