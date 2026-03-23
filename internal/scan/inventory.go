package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const inventoryKQL = `
Resources
| project id, name, type, resourceGroup, location, subscriptionId, properties
| order by resourceGroup asc, type asc`

// ResourceGroup summarises all resources within one Azure resource group.
type ResourceGroup struct {
	Name           string            `json:"name"`
	SubscriptionID string            `json:"subscriptionId"`
	Location       string            `json:"location"`
	ResourceCount  int               `json:"resourceCount"`
	ResourceTypes  []string          `json:"resourceTypes"`
	Resources      []ResourceSummary `json:"resources"`
}

// ResourceSummary is the per-resource data collected during scan.
type ResourceSummary struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	ResourceGroup  string         `json:"resourceGroup"`
	Location       string         `json:"location"`
	SubscriptionID string         `json:"subscriptionId"`
	Properties     map[string]any `json:"properties,omitempty"`
}

// Inventory queries the Azure Resource Graph and returns a map of resource
// group name → ResourceGroup, covering all resources in the subscription.
func Inventory(ctx context.Context, client Client, subscriptionID string) (map[string]*ResourceGroup, error) {
	rows, err := client.Query(ctx, []string{subscriptionID}, inventoryKQL)
	if err != nil {
		return nil, fmt.Errorf("inventory query: %w", err)
	}

	groups := map[string]*ResourceGroup{}

	for _, row := range rows {
		res := rowToResource(row)
		rg, ok := groups[res.ResourceGroup]
		if !ok {
			rg = &ResourceGroup{
				Name:           res.ResourceGroup,
				SubscriptionID: res.SubscriptionID,
				Location:       res.Location,
			}
			groups[res.ResourceGroup] = rg
		}
		rg.Resources = append(rg.Resources, res)
		rg.ResourceCount++
	}

	for _, rg := range groups {
		rg.ResourceTypes = uniqueTypes(rg.Resources)
	}

	return groups, nil
}

// PrintTable renders the inventory as a Unicode box-drawing table to w.
func PrintTable(w io.Writer, groups map[string]*ResourceGroup) {
	const (
		colRG    = 36
		colCount = 7
		colTypes = 48
	)

	hr := func(l, m, r, h string) string {
		return l + strings.Repeat(h, colRG+2) + m +
			strings.Repeat(h, colCount+2) + m +
			strings.Repeat(h, colTypes+2) + r + "\n"
	}
	row := func(rg, count, types string) string {
		return fmt.Sprintf("│ %-*s │ %-*s │ %-*s │\n",
			colRG, rg, colCount, count, colTypes, types)
	}

	_, _ = fmt.Fprint(w, hr("┌", "┬", "┐", "─"))
	_, _ = fmt.Fprint(w, row("Resource Group", "Count", "Resource Types"))
	_, _ = fmt.Fprint(w, hr("├", "┼", "┤", "─"))

	for _, name := range sortedKeys(groups) {
		rg := groups[name]
		types := truncateList(rg.ResourceTypes, 4)
		_, _ = fmt.Fprint(w, row(name, fmt.Sprintf("%d", rg.ResourceCount), types))
	}
	_, _ = fmt.Fprint(w, hr("└", "┴", "┘", "─"))
}

// PrintJSON serialises the inventory to w as indented JSON.
func PrintJSON(w io.Writer, groups map[string]*ResourceGroup) error {
	list := make([]*ResourceGroup, 0, len(groups))
	for _, name := range sortedKeys(groups) {
		list = append(list, groups[name])
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(list)
}

// rowToResource maps a raw Resource Graph row to a ResourceSummary.
func rowToResource(row map[string]any) ResourceSummary {
	r := ResourceSummary{
		ID:             str(row["id"]),
		Name:           str(row["name"]),
		Type:           strings.ToLower(str(row["type"])),
		ResourceGroup:  strings.ToLower(str(row["resourceGroup"])),
		Location:       str(row["location"]),
		SubscriptionID: str(row["subscriptionId"]),
	}
	if props, ok := row["properties"].(map[string]any); ok {
		r.Properties = props
	}
	return r
}

func str(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func uniqueTypes(resources []ResourceSummary) []string {
	seen := map[string]struct{}{}
	for _, r := range resources {
		seen[r.Type] = struct{}{}
	}
	types := make([]string, 0, len(seen))
	for t := range seen {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

func sortedKeys(m map[string]*ResourceGroup) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func truncateList(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(", ... (+%d)", len(items)-max)
}
