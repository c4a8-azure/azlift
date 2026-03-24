package refine

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

var initPos = hcl.Pos{Line: 1, Column: 1}

// GroupMap maps a target filename (without directory) to the set of
// azurerm resource type prefixes that belong in it. The map is the
// canonical grouping table — add new entries here to extend without
// changing any other code.
var GroupMap = map[string][]string{
	"networking.tf": {
		"azurerm_virtual_network",
		"azurerm_subnet",
		"azurerm_network_security_group",
		"azurerm_network_security_rule",
		"azurerm_route_table",
		"azurerm_route",
		"azurerm_public_ip",
		"azurerm_public_ip_prefix",
		"azurerm_virtual_network_peering",
		"azurerm_local_network_gateway",
		"azurerm_virtual_network_gateway",
		"azurerm_application_gateway",
		"azurerm_firewall",
		"azurerm_firewall_policy",
		"azurerm_firewall_policy_rule_collection_group",
		"azurerm_virtual_hub",
		"azurerm_vpn_gateway",
		"azurerm_express_route_circuit",
		"azurerm_private_endpoint",
		"azurerm_private_dns_zone",
		"azurerm_private_dns_zone_virtual_network_link",
		"azurerm_network_interface",
		"azurerm_lb",
		"azurerm_lb_backend_address_pool",
		"azurerm_lb_rule",
		"azurerm_lb_probe",
	},
	"compute.tf": {
		"azurerm_virtual_machine",
		"azurerm_linux_virtual_machine",
		"azurerm_windows_virtual_machine",
		"azurerm_virtual_machine_scale_set",
		"azurerm_linux_virtual_machine_scale_set",
		"azurerm_windows_virtual_machine_scale_set",
		"azurerm_managed_disk",
		"azurerm_virtual_machine_data_disk_attachment",
		"azurerm_availability_set",
		"azurerm_proximity_placement_group",
		"azurerm_image",
		"azurerm_shared_image_gallery",
		"azurerm_shared_image",
		"azurerm_shared_image_version",
	},
	"data.tf": {
		"azurerm_storage_account",
		"azurerm_storage_container",
		"azurerm_storage_blob",
		"azurerm_storage_queue",
		"azurerm_storage_table",
		"azurerm_storage_share",
		"azurerm_sql_server",
		"azurerm_sql_database",
		"azurerm_mssql_server",
		"azurerm_mssql_database",
		"azurerm_mssql_elasticpool",
		"azurerm_cosmosdb_account",
		"azurerm_cosmosdb_sql_database",
		"azurerm_cosmosdb_sql_container",
		"azurerm_postgresql_server",
		"azurerm_postgresql_database",
		"azurerm_postgresql_flexible_server",
		"azurerm_mysql_server",
		"azurerm_mysql_database",
		"azurerm_mysql_flexible_server",
		"azurerm_redis_cache",
		"azurerm_data_factory",
		"azurerm_synapse_workspace",
		"azurerm_service_bus_namespace",
		"azurerm_service_bus_queue",
		"azurerm_service_bus_topic",
		"azurerm_eventhub_namespace",
		"azurerm_eventhub",
	},
	"keyvault.tf": {
		"azurerm_key_vault",
		"azurerm_key_vault_key",
		"azurerm_key_vault_secret",
		"azurerm_key_vault_certificate",
		"azurerm_key_vault_access_policy",
	},
	"iam.tf": {
		"azurerm_user_assigned_identity",
		"azurerm_role_assignment",
		"azurerm_role_definition",
		"azurerm_policy_assignment",
		"azurerm_policy_definition",
	},
	"monitoring.tf": {
		"azurerm_log_analytics_workspace",
		"azurerm_monitor_diagnostic_setting",
		"azurerm_monitor_action_group",
		"azurerm_monitor_metric_alert",
		"azurerm_monitor_log_profile",
		"azurerm_application_insights",
	},
	"appservice.tf": {
		"azurerm_app_service_plan",
		"azurerm_service_plan",
		"azurerm_app_service",
		"azurerm_linux_web_app",
		"azurerm_windows_web_app",
		"azurerm_linux_function_app",
		"azurerm_windows_function_app",
		"azurerm_function_app",
		"azurerm_container_registry",
		"azurerm_kubernetes_cluster",
		"azurerm_kubernetes_cluster_node_pool",
		"azurerm_api_management",
	},
}

// resourceToFile is the inverted index built once at package init.
var resourceToFile map[string]string

func init() {
	resourceToFile = make(map[string]string, 128)
	for filename, types := range GroupMap {
		for _, rt := range types {
			resourceToFile[rt] = filename
		}
	}
}

// targetFile returns the output filename for a resource type label.
// Falls back to "main.tf" for unknown types.
func targetFile(resourceType string) string {
	if f, ok := resourceToFile[strings.ToLower(resourceType)]; ok {
		return f
	}
	return "main.tf"
}

// GroupResources redistributes resource blocks from the input files into
// new ParsedFiles grouped by logical topic. Each resource block appears
// in exactly one output file.
//
// Non-resource blocks (terraform {}, provider {}, variable {}, …) are
// placed into main.tf to preserve provider configuration.
//
// Returns a slice of non-empty ParsedFiles ready to be written to outputDir.
func GroupResources(files []*ParsedFile, outputDir string) []*ParsedFile {
	// Bucket → file builder.
	buckets := map[string]*hclwrite.File{}

	getOrCreate := func(name string) *hclwrite.File {
		if f, ok := buckets[name]; ok {
			return f
		}
		f := hclwrite.NewEmptyFile()
		buckets[name] = f
		return f
	}

	for _, pf := range files {
		for _, block := range pf.File.Body().Blocks() {
			// Skip blocks that the scaffold generates into separate files:
			//   terraform {}  → versions.tf + backend.tf
			//   provider {}   → providers.tf
			// Keeping them here would cause Terraform to error on duplicate blocks.
			if block.Type() == "terraform" || block.Type() == "provider" {
				continue
			}

			var dest string
			if block.Type() == "resource" && len(block.Labels()) >= 1 {
				dest = targetFile(block.Labels()[0])
			} else {
				dest = "main.tf"
			}
			target := getOrCreate(dest)
			target.Body().AppendBlock(cloneBlock(block))
			target.Body().AppendNewline()
		}
	}

	// Convert to ParsedFile slice, sorted for determinism.
	names := make([]string, 0, len(buckets))
	for n := range buckets {
		names = append(names, n)
	}
	sort.Strings(names)

	var result []*ParsedFile
	for _, name := range names {
		result = append(result, &ParsedFile{
			Path: filepath.Join(outputDir, name),
			File: buckets[name],
		})
	}
	return result
}

// cloneBlock performs a shallow copy of an hclwrite.Block by re-serialising
// its bytes through hclwrite.ParseConfig. This is the canonical way to copy
// blocks across files in hclwrite (the AST nodes are tied to their parent).
func cloneBlock(block *hclwrite.Block) *hclwrite.Block {
	src := block.BuildTokens(nil).Bytes()
	// Wrap in a minimal file so ParseConfig has valid top-level content.
	wrapped := append(src, '\n') //nolint:gocritic // intentional new slice
	f, diags := hclwrite.ParseConfig(wrapped, "<clone>", initPos)
	if diags.HasErrors() {
		// Fall back: return an empty block of the same type/labels.
		return hclwrite.NewBlock(block.Type(), block.Labels())
	}
	blocks := f.Body().Blocks()
	if len(blocks) == 0 {
		return hclwrite.NewBlock(block.Type(), block.Labels())
	}
	return blocks[0]
}
