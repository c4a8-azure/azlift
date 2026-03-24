package enrich

import (
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/c4a8-azure/azlift/internal/refine"
)

// StatefulResourceTypes is the set of Azure resource types that hold
// persistent data and should be protected with prevent_destroy = true.
// Extend this map to cover additional types without changing any other code.
var StatefulResourceTypes = map[string]bool{
	"azurerm_key_vault":                  true,
	"azurerm_key_vault_key":              true,
	"azurerm_storage_account":            true,
	"azurerm_sql_server":                 true,
	"azurerm_mssql_server":               true,
	"azurerm_mssql_database":             true,
	"azurerm_postgresql_server":          true,
	"azurerm_postgresql_flexible_server": true,
	"azurerm_mysql_server":               true,
	"azurerm_mysql_flexible_server":      true,
	"azurerm_cosmosdb_account":           true,
	"azurerm_redis_cache":                true,
	"azurerm_managed_disk":               true,
	"azurerm_recovery_services_vault":    true,
	"azurerm_backup_policy_vm":           true,
	"azurerm_synapse_workspace":          true,
	"azurerm_data_factory":               true,
	"azurerm_service_bus_namespace":      true,
	"azurerm_eventhub_namespace":         true,
	"azurerm_log_analytics_workspace":    true,
}

// InjectPreventDestroy iterates over all resource blocks in the provided
// parsed files and adds `lifecycle { prevent_destroy = true }` to any
// stateful resource that does not already have it.
//
// The files are modified in-place (the underlying hclwrite AST is updated).
// Returns the count of blocks that were modified.
func InjectPreventDestroy(files []*refine.ParsedFile, statefulTypes map[string]bool) int {
	if statefulTypes == nil {
		statefulTypes = StatefulResourceTypes
	}

	modified := 0
	for _, pf := range files {
		for _, block := range refine.Blocks(pf, "resource") {
			if len(block.Labels()) < 1 {
				continue
			}
			resourceType := strings.ToLower(block.Labels()[0])
			if !statefulTypes[resourceType] {
				continue
			}
			if injectLifecycle(block) {
				modified++
			}
		}
	}
	return modified
}

// injectLifecycle adds prevent_destroy = true to the lifecycle block of b.
// If a lifecycle block already exists with prevent_destroy = true, it is
// left unchanged (idempotent). Returns true if the block was modified.
func injectLifecycle(block *hclwrite.Block) bool {
	body := block.Body()

	// Find an existing lifecycle block.
	for _, nested := range body.Blocks() {
		if nested.Type() != "lifecycle" {
			continue
		}
		// lifecycle block exists — check if prevent_destroy is already set.
		if attr := nested.Body().GetAttribute("prevent_destroy"); attr != nil {
			val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
			if val == "true" {
				return false // already set, nothing to do
			}
		}
		nested.Body().SetAttributeValue("prevent_destroy", cty.True)
		return true
	}

	// No lifecycle block — append one.
	lc := body.AppendNewBlock("lifecycle", nil)
	lc.Body().SetAttributeValue("prevent_destroy", cty.True)
	return true
}
