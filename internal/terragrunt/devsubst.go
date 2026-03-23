package terragrunt

import (
	"errors"
	"strings"
)

// SubstitutionRule describes a single SKU / attribute downgrade applied when
// generating the dev environment from a prod export.
type SubstitutionRule struct {
	// AttrName is the HCL attribute name to inspect, e.g. "sku_name".
	AttrName string
	// From is the prod value (case-insensitive substring match).
	From string
	// To is the dev replacement value (exact).
	To string
}

// DefaultSubstitutions is the set of built-in dev/prod SKU downgrades.
// Rules are applied in order; the first matching rule wins.
var DefaultSubstitutions = []SubstitutionRule{
	// Storage: GRS/RAGRS → LRS (locally redundant, cheapest)
	{AttrName: "account_replication_type", From: "GRS", To: "LRS"},
	{AttrName: "account_replication_type", From: "RAGRS", To: "LRS"},
	// Storage / Service Bus / Event Hubs tier
	{AttrName: "account_tier", From: "Premium", To: "Standard"},
	{AttrName: "sku", From: "Premium", To: "Standard"},
	{AttrName: "sku_name", From: "Premium", To: "Standard"},
	// App Service Plan / AKS node pool
	{AttrName: "sku_name", From: "P2", To: "B1"},
	{AttrName: "sku_name", From: "P3", To: "B1"},
	// Redis Cache
	{AttrName: "family", From: "P", To: "C"},
	{AttrName: "sku_name", From: "Premium", To: "Basic"},
	// SQL / PostgreSQL
	{AttrName: "sku_name", From: "GP_", To: "B_"},
	{AttrName: "sku_name", From: "MO_", To: "B_"},
	// Zone redundancy — set to false / single zone
	{AttrName: "zone_redundant", From: "true", To: "false"},
	{AttrName: "zones", From: "[", To: `["1"]`},
}

// ApplyDevSubstitutions takes an inputs map (attribute name → HCL literal
// value string) and returns a new map with dev-appropriate substitutions
// applied according to rules. The original map is not modified.
func ApplyDevSubstitutions(inputs map[string]string, rules []SubstitutionRule) map[string]string {
	if len(rules) == 0 {
		rules = DefaultSubstitutions
	}

	out := make(map[string]string, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}

	for attrName, val := range out {
		for _, rule := range rules {
			if !strings.EqualFold(attrName, rule.AttrName) {
				continue
			}
			// Case-insensitive substring match on the unquoted value.
			bare := strings.Trim(val, `"`)
			if strings.Contains(strings.ToLower(bare), strings.ToLower(rule.From)) {
				// Rebuild with the dev replacement, preserving surrounding quotes if present.
				replaced := strings.ReplaceAll(bare, rule.From, rule.To)
				if strings.HasPrefix(val, `"`) {
					out[attrName] = `"` + replaced + `"`
				} else {
					out[attrName] = replaced
				}
				break // first matching rule wins
			}
		}
	}
	return out
}

// DowngradeInstanceCount reduces numeric capacity / instance count attributes
// to 1 for dev environments when the prod value is > 2.
func DowngradeInstanceCount(inputs map[string]string) map[string]string {
	out := make(map[string]string, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}

	capacityAttrs := map[string]bool{
		"capacity":       true,
		"instance_count": true,
		"node_count":     true,
		"max_replicas":   true,
	}

	for attr := range capacityAttrs {
		if val, ok := out[attr]; ok {
			// Only downgrade if value is a plain integer > 2.
			var n int
			if _, err := scanInt(val, &n); err == nil && n > 2 {
				out[attr] = "1"
			}
		}
	}
	return out
}

// scanInt parses an integer from a string literal (handles bare integers and
// quoted integers like "3").
func scanInt(s string, out *int) (int, error) {
	s = strings.Trim(s, `"`)
	n := 0
	if _, err := parseInt(s, &n); err != nil {
		return 0, err
	}
	*out = n
	return n, nil
}

func parseInt(s string, out *int) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotInt
		}
		n = n*10 + int(c-'0')
	}
	if len(s) == 0 {
		return 0, errNotInt
	}
	*out = n
	return n, nil
}

var errNotInt = errors.New("not an integer")
