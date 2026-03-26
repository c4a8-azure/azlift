package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModuleConfig holds the parameters for generating the bootstrap/ Terraform module.
type ModuleConfig struct {
	// RepoName is the GitHub repository name.
	RepoName string
	// RepoOrg is the GitHub organisation.
	RepoOrg string
	// Environments is the list of deployment tiers.
	Environments []string
	// SubscriptionID is the target Azure subscription.
	SubscriptionID string
	// TenantID is the target Azure AD tenant.
	TenantID string
	// Location is the Azure region for state storage.
	Location string
	// ResourceGroups are the RGs whose resources are being managed.
	// Used as RBAC scope in same-tenant mode.
	ResourceGroups []string
	// IsCrossTenant, when true, scopes RBAC to the subscription instead of individual RGs.
	IsCrossTenant bool
	// StateStorage holds the derived names for the storage account / container.
	StateStorage StateStorageConfig
}

// GenerateBootstrapModule writes a self-contained bootstrap/ Terraform module
// into repoDir. The module provisions all resources required to activate CI/CD:
// state storage, Managed Identities, OIDC federated credentials, RBAC, and
// GitHub environment variables.
func GenerateBootstrapModule(cfg ModuleConfig, repoDir string) error {
	dir := filepath.Join(repoDir, "bootstrap")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating bootstrap dir: %w", err)
	}

	files := map[string]string{
		"terraform.tf": generateTerraformTF(),
		"variables.tf": generateVariablesTF(cfg),
		"main.tf":      generateMainTF(cfg),
		"outputs.tf":   generateOutputsTF(cfg),
	}

	for name, content := range files {
		dest := filepath.Join(dir, name)
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("writing bootstrap/%s: %w", name, err)
		}
	}
	return nil
}

func generateTerraformTF() string {
	return `terraform {
  required_version = ">= 1.5"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

provider "github" {
  owner = var.github_org
}
`
}

func generateVariablesTF(cfg ModuleConfig) string {
	envList := `["` + strings.Join(cfg.Environments, `", "`) + `"]`
	rgList := buildRGList(cfg.ResourceGroups)
	return fmt.Sprintf(`variable "subscription_id" {
  description = "Target Azure subscription ID."
  type        = string
  default     = %q
}

variable "tenant_id" {
  description = "Target Azure AD tenant ID."
  type        = string
  default     = %q
}

variable "location" {
  description = "Azure region for state storage resources."
  type        = string
  default     = %q
}

variable "environments" {
  description = "Deployment tiers to configure (plan + apply MI per env)."
  type        = list(string)
  default     = %s
}

variable "github_org" {
  description = "GitHub organisation that owns the repository."
  type        = string
  default     = %q
}

variable "github_repo" {
  description = "GitHub repository name."
  type        = string
  default     = %q
}

variable "managed_rgs" {
  description = "Resource groups whose resources are being managed (used for RBAC scope)."
  type        = list(string)
  default     = %s
}
`,
		cfg.SubscriptionID,
		cfg.TenantID,
		cfg.Location,
		envList,
		cfg.RepoOrg,
		cfg.RepoName,
		rgList,
	)
}

func generateMainTF(cfg ModuleConfig) string {
	stateCfg := cfg.StateStorage
	rbacScope := buildRBACScope(cfg)

	return fmt.Sprintf(`# Apply this module once to activate CI/CD for this repository.
#
# Prerequisites:
#   az login (--tenant %s for cross-tenant)
#   gh auth login
#
# Usage:
#   terraform -chdir=bootstrap init
#   terraform -chdir=bootstrap apply

locals {
  env_roles = flatten([
    for env in var.environments : [
      { env = env, role = "plan" },
      { env = env, role = "apply" },
    ]
  ])
  # Map "env/role" -> MI name
  mi_keys = { for er in local.env_roles : "${er.env}/${er.role}" => er }
}

# ── State storage ────────────────────────────────────────────────────────────

resource "azurerm_resource_group" "state" {
  name     = %q
  location = var.location
}

resource "azurerm_storage_account" "state" {
  name                            = %q
  resource_group_name             = azurerm_resource_group.state.name
  location                        = azurerm_resource_group.state.location
  account_tier                    = "Standard"
  account_replication_type        = "LRS"
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false
}

resource "azurerm_storage_container" "tfstate" {
  name                  = %q
  storage_account_id    = azurerm_storage_account.state.id
  container_access_type = "private"
}

# ── Managed Identities ───────────────────────────────────────────────────────

resource "azurerm_user_assigned_identity" "mi" {
  for_each            = local.mi_keys
  name                = "mi-%s-${each.value.env}-${each.value.role}"
  resource_group_name = azurerm_resource_group.state.name
  location            = azurerm_resource_group.state.location
}

# ── OIDC federated credentials ───────────────────────────────────────────────

resource "azurerm_federated_identity_credential" "plan" {
  for_each            = { for env in var.environments : env => env }
  name                = "fc-${each.key}-plan"
  resource_group_name = azurerm_resource_group.state.name
  parent_id           = azurerm_user_assigned_identity.mi["${each.key}/plan"].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = "https://token.actions.githubusercontent.com"
  subject             = "repo:${var.github_org}/${var.github_repo}:pull_request"
}

resource "azurerm_federated_identity_credential" "apply" {
  for_each            = { for env in var.environments : env => env }
  name                = "fc-${each.key}-apply"
  resource_group_name = azurerm_resource_group.state.name
  parent_id           = azurerm_user_assigned_identity.mi["${each.key}/apply"].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = "https://token.actions.githubusercontent.com"
  subject             = "repo:${var.github_org}/${var.github_repo}:ref:refs/heads/main"
}

# ── RBAC ─────────────────────────────────────────────────────────────────────
%s

# ── GitHub environments ───────────────────────────────────────────────────────

locals {
  gh_envs = flatten([
    for env in var.environments : [
      "${env}-iac-plan",
      "${env}-iac-apply",
    ]
  ])
}

resource "github_repository_environment" "env" {
  for_each    = toset(local.gh_envs)
  repository  = var.github_repo
  environment = each.key
}

resource "github_actions_environment_variable" "client_id" {
  for_each      = local.mi_keys
  repository    = var.github_repo
  environment   = "${each.value.env}-iac-${each.value.role}"
  variable_name = "AZURE_CLIENT_ID"
  value         = azurerm_user_assigned_identity.mi[each.key].client_id

  depends_on = [github_repository_environment.env]
}

resource "github_actions_environment_variable" "tenant_id" {
  for_each      = local.mi_keys
  repository    = var.github_repo
  environment   = "${each.value.env}-iac-${each.value.role}"
  variable_name = "AZURE_TENANT_ID"
  value         = var.tenant_id

  depends_on = [github_repository_environment.env]
}

resource "github_actions_environment_variable" "subscription_id" {
  for_each      = local.mi_keys
  repository    = var.github_repo
  environment   = "${each.value.env}-iac-${each.value.role}"
  variable_name = "AZURE_SUBSCRIPTION_ID"
  value         = var.subscription_id

  depends_on = [github_repository_environment.env]
}
`,
		cfg.TenantID,
		stateCfg.ResourceGroupName,
		stateCfg.StorageAccountName,
		stateCfg.ContainerName,
		sanitiseRepoName(cfg.RepoName),
		rbacScope,
	)
}

func generateOutputsTF(cfg ModuleConfig) string {
	_ = cfg // reserved for future per-env outputs
	return `output "state_storage_account" {
  description = "Name of the Terraform state storage account."
  value       = azurerm_storage_account.state.name
}

output "state_container" {
  description = "Name of the Terraform state blob container."
  value       = azurerm_storage_container.tfstate.name
}

output "managed_identity_client_ids" {
  description = "Map of env/role → MI client ID."
  value       = { for k, mi in azurerm_user_assigned_identity.mi : k => mi.client_id }
}
`
}

// buildRBACScope generates the azurerm_role_assignment blocks.
// Same-tenant: scoped to each managed resource group.
// Cross-tenant: scoped to the subscription.
func buildRBACScope(cfg ModuleConfig) string {
	if cfg.IsCrossTenant || len(cfg.ResourceGroups) == 0 {
		return `
resource "azurerm_role_assignment" "plan" {
  for_each             = { for env in var.environments : env => env }
  scope                = "/subscriptions/${var.subscription_id}"
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.mi["${each.key}/plan"].principal_id
}

resource "azurerm_role_assignment" "apply" {
  for_each             = { for env in var.environments : env => env }
  scope                = "/subscriptions/${var.subscription_id}"
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.mi["${each.key}/apply"].principal_id
}`
	}

	// Same-tenant: one role assignment per env × RG.
	return `
locals {
  env_rg_plan = {
    for pair in setproduct(var.environments, var.managed_rgs) :
    "${pair[0]}/${pair[1]}" => { env = pair[0], rg = pair[1] }
  }
}

resource "azurerm_role_assignment" "plan" {
  for_each             = local.env_rg_plan
  scope                = "/subscriptions/${var.subscription_id}/resourceGroups/${each.value.rg}"
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.mi["${each.value.env}/plan"].principal_id
}

resource "azurerm_role_assignment" "apply" {
  for_each             = local.env_rg_plan
  scope                = "/subscriptions/${var.subscription_id}/resourceGroups/${each.value.rg}"
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.mi["${each.value.env}/apply"].principal_id
}`
}

func buildRGList(rgs []string) string {
	if len(rgs) == 0 {
		return "[]"
	}
	quoted := make([]string, len(rgs))
	for i, rg := range rgs {
		quoted[i] = fmt.Sprintf("%q", rg)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
