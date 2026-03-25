package refine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// noTagsResourceTypes is the deny-list of azurerm resource types that do NOT
// support a `tags` argument. These are typically child/sub-resources,
// association resources, data-plane resources, or configuration-only resources.
//
// Rule: when a new resource type causes "An argument named 'tags' is not
// expected here", add it here. Keep entries grouped and sorted within groups.
var noTagsResourceTypes = map[string]bool{
	// ── Networking: sub-resources and associations ────────────────────────────
	"azurerm_network_interface_application_gateway_backend_address_pool_association": true,
	"azurerm_network_interface_backend_address_pool_association":                     true,
	"azurerm_network_interface_nat_rule_association":                                 true,
	"azurerm_network_interface_security_group_association":                           true,
	"azurerm_network_security_rule":                                                  true,
	"azurerm_route":                                                                  true,
	"azurerm_subnet":                                                                 true,
	"azurerm_subnet_network_security_group_association":                              true,
	"azurerm_subnet_route_table_association":                                         true,
	"azurerm_subnet_service_endpoint_storage_policy":                                 true,
	"azurerm_virtual_network_peering":                                                true,

	// ── Storage: data-plane and configuration resources ───────────────────────
	"azurerm_storage_account_blob_properties":          true,
	"azurerm_storage_account_network_rules":            true,
	"azurerm_storage_account_queue_properties":         true,
	"azurerm_storage_blob":                             true,
	"azurerm_storage_container":                        true,
	"azurerm_storage_management_policy":                true,
	"azurerm_storage_object_replication":               true,
	"azurerm_storage_queue":                            true,
	"azurerm_storage_share":                            true,
	"azurerm_storage_share_directory":                  true,
	"azurerm_storage_table":                            true,
	"azurerm_storage_table_entity":                     true,

	// ── IAM / identity ────────────────────────────────────────────────────────
	"azurerm_federated_identity_credential": true,
	"azurerm_role_assignment":               true,

	// ── DNS records (zone and link resources DO have tags) ────────────────────
	"azurerm_private_dns_cname_record":  true,
	"azurerm_private_dns_ptr_record":    true,
	"azurerm_private_dns_srv_record":    true,
	"azurerm_private_dns_txt_record":    true,

	// ── App Service / Function: association and slot config resources ──────────
	"azurerm_app_service_virtual_network_swift_connection": true,
	"azurerm_function_app_function":                        true,

	// ── Locks and policy assignments ──────────────────────────────────────────
	"azurerm_management_lock": true,

	// ── Kubernetes: node pool associations ────────────────────────────────────
	"azurerm_kubernetes_fleet_member": true,

	// ── Load balancer sub-resources ───────────────────────────────────────────
	"azurerm_lb_backend_address_pool_address": true,
	"azurerm_lb_nat_rule":                     true,
	"azurerm_lb_outbound_rule":                true,
	"azurerm_lb_probe":                        true,
	"azurerm_lb_rule":                         true,
}

// StandardTagKeys are the tag keys that must appear in local.common_tags.
var StandardTagKeys = []string{
	"environment",
	"workload",
	"managed-by",
	"created-by",
	"cost-center",
}

// CommonTagsLocalName is the local name used for the shared tags map.
const CommonTagsLocalName = "common_tags"

// NormaliseTags performs two operations:
//
//  1. Injects a `common_tags` entry into the `locals {}` block of the
//     locals file, containing the tag keys with empty-string defaults
//     (the caller / environment will supply real values).
//  2. Rewrites every `tags = { ... }` attribute on resource blocks to use
//     `merge(local.common_tags, { <bespoke> })`, preserving any tags that
//     are not part of the standard set.
//
// The optional tagKeys parameter overrides StandardTagKeys. When empty,
// StandardTagKeys is used unchanged.
//
// Returns the count of resource blocks whose tags were normalised.
func NormaliseTags(files []*ParsedFile, localsFile *ParsedFile, tagKeys ...string) int {
	keys := StandardTagKeys
	if len(tagKeys) > 0 {
		keys = tagKeys
	}
	injectCommonTagsLocal(localsFile, keys)

	normalised := 0
	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			labels := block.Labels()
			if len(labels) > 0 && noTagsResourceTypes[labels[0]] {
				continue
			}
			if normaliseTagsBlock(block) {
				normalised++
			}
		}
	}
	return normalised
}

// injectCommonTagsLocal adds (or overwrites) the common_tags attribute in
// the first locals {} block of localsFile using the provided tag keys.
func injectCommonTagsLocal(pf *ParsedFile, keys []string) {
	var localsBlock *hclwrite.Block
	for _, b := range pf.File.Body().Blocks() {
		if b.Type() == "locals" {
			localsBlock = b
			break
		}
	}
	if localsBlock == nil {
		localsBlock = pf.File.Body().AppendNewBlock("locals", nil)
	}

	localsBlock.Body().SetAttributeRaw(CommonTagsLocalName, hclwrite.TokensForIdentifier(buildCommonTagsObject(keys)))
}

func buildCommonTagsObject(keys []string) string {
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	var sb strings.Builder
	sb.WriteString("{\n")
	for _, k := range sorted {
		fmt.Fprintf(&sb, "    %s = \"\"\n", k)
	}
	sb.WriteString("  }")
	return sb.String()
}

// normaliseTagsBlock rewrites the `tags` attribute of a resource block to use
// merge(local.common_tags, {...}). Returns true if the block was modified.
func normaliseTagsBlock(block *hclwrite.Block) bool {
	attr := block.Body().GetAttribute("tags")
	if attr == nil {
		block.Body().SetAttributeRaw("tags",
			hclwrite.TokensForIdentifier(fmt.Sprintf("merge(local.%s, {})", CommonTagsLocalName)))
		return true
	}

	val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))

	// Already normalised — idempotent.
	if strings.HasPrefix(val, fmt.Sprintf("merge(local.%s,", CommonTagsLocalName)) ||
		strings.HasPrefix(val, fmt.Sprintf("merge(local.%s ,", CommonTagsLocalName)) {
		return false
	}

	bespoke := extractBespokeTags(val)
	block.Body().SetAttributeRaw("tags",
		hclwrite.TokensForIdentifier(fmt.Sprintf("merge(local.%s, %s)", CommonTagsLocalName, bespoke)))
	return true
}

// extractBespokeTags filters out the standard tag keys from a literal object
// expression and returns the remainder as an HCL object literal.
// When there are no bespoke keys, returns "{}".
func extractBespokeTags(val string) string {
	if !strings.HasPrefix(val, "{") {
		return val
	}

	standardSet := map[string]bool{}
	for _, k := range StandardTagKeys {
		standardSet[k] = true
	}

	inner := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(val), "}"), "{")
	var bespoke []string
	for _, line := range strings.Split(inner, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || (strings.HasSuffix(line, ",") && len(line) == 1) {
			continue
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			bespoke = append(bespoke, line)
			continue
		}
		key := strings.Trim(strings.TrimSpace(line[:eqIdx]), `"`)
		if !standardSet[key] {
			bespoke = append(bespoke, strings.TrimSuffix(line, ","))
		}
	}

	if len(bespoke) == 0 {
		return "{}"
	}
	return "{\n    " + strings.Join(bespoke, "\n    ") + "\n  }"
}
