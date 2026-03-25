# azlift

> Turn "what was clicked" into clean, auditable, pipeline-driven infrastructure — without starting from scratch.

A wrapper tool that orchestrates [aztfexport](https://github.com/Azure/aztfexport) into a single, opinionated pipeline that converts portal-created Azure resources into production-ready Terraform or Terragrunt code, fully wired into a Git-based CI/CD setup. The bootstrap stage provisions state storage, Managed Identities, OIDC, and GitHub pipelines natively — no PowerShell modules required.

---

## The Problem

ClickOps is a reality. Resources get provisioned through the Azure Portal under time pressure, during proof-of-concepts, or simply because it was faster. The result is technical debt: no Infrastructure-as-Code, no CI/CD, no auditable history, and no clean path to a GitOps workflow.

Ignoring it is not an option. Every resource provisioned outside of code is a resource that can be misconfigured, duplicated, or lost.

`aztfexport` solves the hard part — extracting Terraform code and state from existing Azure resources. But its raw output is not production-ready:

- One monolithic `main.tf` with hundreds of resources
- Hardcoded values everywhere: resource IDs, locations, names
- No variables, no locals, no outputs
- No logical grouping or module structure
- Nothing DRY — every repeated pattern is copy-pasted
- No backend configuration, no CI/CD, no repository

`azlift` closes that gap. It takes aztfexport's raw output and transforms it into structured, maintainable Terraform or Terragrunt code, then bootstraps a complete GitOps-ready repository around it.

---

## How It Works

```
┌─────────────────────────────────────────────────────────────────────┐
│                        azlift                              │
│                                                                     │
│  ┌─────────┐   ┌──────────┐   ┌───────────┐   ┌─────────────────┐ │
│  │  SCAN   │──▶│  EXPORT  │──▶│  REFINE   │──▶│    BOOTSTRAP    │ │
│  └─────────┘   └──────────┘   └───────────┘   └─────────────────┘ │
│  Azure Graph   aztfexport      Transform        Native Go +         │
│  API           (wrapped)       Engine           gh CLI              │
└─────────────────────────────────────────────────────────────────────┘
         ↓              ↓               ↓                  ↓
    Resource      raw .tf +        structured          Git repo +
    inventory     .tfstate         Terraform or        OIDC MIs +
    + RG map                       Terragrunt          pipelines
```

### Stage 1 — SCAN

Before exporting anything, the tool builds a resource inventory and cross-resource dependency graph using the Azure Resource Graph API.

```
azlift scan --subscription <id>

┌────────────────────────────┬───────┬─────────────────────────────┐
│ Resource Group             │ Count │ Resource Types               │
├────────────────────────────┼───────┼─────────────────────────────┤
│ rg-prod-app-westeu         │   23  │ VM, NIC, NSG, Disk, ...     │
│ rg-prod-network-westeu     │    8  │ VNet, Subnet, GW, ...       │
│ rg-prod-data-westeu        │   12  │ SQL, Storage, Key Vault, ... │
└────────────────────────────┴───────┴─────────────────────────────┘

Dependency analysis:
  rg-prod-app-westeu     → rg-prod-network-westeu  (VNet references)
  rg-prod-app-westeu     → rg-prod-data-westeu     (Key Vault references)

Recommendation: Export as 3 separate Terraform roots with cross-root data sources.
```

This is something aztfexport does not do. By analyzing dependencies before exporting, the tool decides where to draw module and state boundaries — so you don't end up with one giant state file or split resources that depend on each other into disconnected modules.

### Stage 2 — EXPORT

The tool wraps aztfexport per logical boundary (typically per resource group), adding:

- Retry logic for API throttling
- Exclusion lists for resources that should not be in IaC (diagnostic settings, certain locks, resource-specific role assignments already managed elsewhere)
- Mapping of unsupported resources to `data` sources rather than silently dropping them
- Structured output directory per boundary

### Stage 3 — REFINE

This is where the transformation happens. Raw aztfexport HCL is parsed and restructured through a multi-step pipeline.

#### Step 1: Variable Extraction

Scans for repeated literal values and extracts them:

```hcl
# Before
resource "azurerm_virtual_network" "example" {
  location            = "westeurope"
  resource_group_name = "rg-prod-network-westeu"
  ...
}

resource "azurerm_subnet" "example" {
  resource_group_name  = "rg-prod-network-westeu"
  virtual_network_name = azurerm_virtual_network.example.name
  ...
}

# After
variable "location" {
  description = "Azure region for all resources."
  type        = string
  default     = "westeurope"
}

locals {
  resource_group_name = "rg-prod-network-westeu"
}
```

#### Step 2: Semantic Analysis

Resource names carry meaning. The tool decodes it:

```
"rg-prod-network-westeu-001"
 │    │       │        │  └── suffix
 │    │       │        └───── region
 │    │       └────────────── workload
 │    └────────────────────── environment
 └─────────────────────────── resource type prefix

→ var.environment  = "prod"
→ var.location     = "westeurope"
→ local.prefix     = "network"
→ local.naming     = "${var.environment}-${local.prefix}-${var.location}"
```

This produces code that reads like it was written intentionally, not generated.

#### Step 3: Resource Grouping

Instead of one flat `main.tf`, resources are split into logical files:

```
networking.tf   — VNet, Subnet, NSG, Route Table
compute.tf      — VM, NIC, Managed Disk, Availability Set
data.tf         — SQL Server, Storage Account
keyvault.tf     — Key Vault, Access Policies
outputs.tf      — Values referenced by other roots
variables.tf    — All input variables
locals.tf       — Derived naming and configuration
versions.tf     — Required providers, pinned versions
backend.tf      — Azure Storage state backend
```

#### Step 4: Backend and Provider Generation

Auto-generates the backend configuration pointing to the state storage account provisioned during bootstrap:

```hcl
terraform {
  backend "azurerm" {
    resource_group_name  = "rg-tfstate-prod"
    storage_account_name = "sttfstateprod001"
    container_name       = "tfstate"
    key                  = "network/terraform.tfstate"
  }
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
}
```

### Stage 3 (alt) — REFINE with Terragrunt (`--mode terragrunt`)

For teams that need multi-environment promotion from day one, the Terragrunt mode generates a layered structure instead of flat modules:

```
output/
├── terragrunt.hcl                  # root: provider + backend config, defined once
├── _envcommon/
│   └── networking.hcl              # shared inputs across all environments
├── dev/
│   ├── env.hcl                     # dev-specific overrides (SKUs, instance counts)
│   ├── networking/
│   │   └── terragrunt.hcl          # inherits from _envcommon/networking.hcl
│   └── compute/
│       └── terragrunt.hcl
└── prod/
    ├── env.hcl                     # prod settings: HA, larger SKUs, geo-redundancy
    ├── networking/
    └── compute/
```

The tool exports from the existing prod environment, then generates a sensible `dev/` configuration by inferring production indicators (SKU names, instance counts, zone settings) and substituting cost-appropriate equivalents.

### Stage 3 (opt) — AI Enrichment (`--enrich`)

After the deterministic transformation, an optional AI enrichment pass reviews each generated file and applies a layer of judgment that static analysis cannot:

- Adds `lifecycle { prevent_destroy = true }` to stateful resources (databases, Key Vaults)
- Flags security anti-patterns: public blob access, missing encryption, open NSG rules
- Writes variable and output descriptions
- Replaces magic numbers with named locals
- Normalizes tag structures to a consistent policy

This maps directly to the "automated fine-tuning and best-practice alignment" goal — the output looks like a senior engineer reviewed it, not like it came out of an export tool.

### Stage 4 — BOOTSTRAP

The native Go bootstrap pipeline wires everything together without any external PowerShell modules:

1. Creates an Azure Resource Group for state storage and Managed Identities
2. Provisions state storage (Standard_LRS, TLS 1.2, HTTPS only, blob public access disabled)
3. Provisions two Managed Identities per environment: one for `plan` (Reader), one for `apply` (Contributor)
4. Configures OIDC federated credentials — no stored secrets, no service principal passwords
5. Initialises a local Git repository with the refined Terraform, GitHub Actions workflows, and a `bootstrap/` Terraform module
6. Creates the GitHub repository via `gh` CLI and pushes
7. Configures GitHub environments (`dev-iac-plan`, `dev-iac-apply`, etc.) with `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, and `AZURE_SUBSCRIPTION_ID` variables
8. Uploads the aztfexport `terraform.tfstate` to the remote state container

**Cross-tenant mode**: when `--target-tenant` differs from the source tenant, Azure provisioning is skipped. A `bootstrap/` Terraform module is generated for the operator to apply manually in the target tenant instead.

The result: `terraform plan` works on the first pipeline run.

---

## Usage

### The "Easy Button" — Full Pipeline

```bash
azlift run \
  --subscription $ARM_SUBSCRIPTION_ID \
  --resource-group rg-prod-westeu \
  --repo-name infra-prod-network \
  --mode terragrunt \
  --enrich \
  --platform github
```

### Step by Step

```bash
# 1. Inventory what exists and analyze dependencies
azlift scan --subscription <subscription-id>

# 2. Export a resource group (wraps aztfexport)
azlift export \
  --resource-group rg-prod-network-westeu \
  --output-dir ./raw

# 3. Transform raw HCL into structured Terraform
azlift refine \
  --input-dir ./raw \
  --mode modules          # or: terragrunt
  --enrich                # optional: AI enrichment pass

# 4. Bootstrap the Git repo and CI/CD
azlift bootstrap \
  --input-dir ./refined \
  --repo-name infra-prod-network \
  --org my-github-org \
  --environments dev,staging,prod
```

### Emergency / Incident Mode

No time for a full bootstrap? Use `--no-bootstrap` to generate code and a Terraform plan immediately — useful during an incident when you need to understand what's deployed or reconstruct a deleted resource:

```bash
azlift run \
  --subscription $ARM_SUBSCRIPTION_ID \
  --resource-group rg-prod-app \
  --mode modules \
  --no-bootstrap \
  --dry-run
```

The output is a Terraform plan showing the current state of the subscription, ready to use for incident analysis or manual reconstruction — before you care about repositories or pipelines.

---

## Output Structure

### Terraform Modules Mode

```
infra-prod-network/
├── .github/
│   └── workflows/
│       ├── tf-plan.yml         # runs on PR
│       └── tf-apply.yml        # runs on merge to main
├── backend.tf
├── versions.tf
├── variables.tf
├── locals.tf
├── networking.tf
├── compute.tf
├── keyvault.tf
├── outputs.tf
├── terraform.tfvars.example
└── bootstrap/                  # Terraform module to activate CI/CD (cross-tenant or manual use)
```

### Terragrunt Mode

```
infra-prod/
├── .github/
│   └── workflows/
│       ├── tg-plan.yml
│       └── tg-apply.yml
├── terragrunt.hcl              # root config
├── _envcommon/
├── dev/
│   ├── env.hcl
│   ├── networking/
│   └── compute/
├── staging/
│   ├── env.hcl
│   ├── networking/
│   └── compute/
└── prod/
    ├── env.hcl
    ├── networking/
    └── compute/
```

---

## Key Design Decisions

### State Splitting Strategy

State boundaries are defined per resource group, not per resource type or as a single monolith. This aligns with Azure RBAC boundaries, limits blast radius, and maps naturally to team ownership.

Cross-root references are handled via `data` sources and Terraform remote state outputs — the scan stage identifies these dependencies before any code is generated.

### State Adoption Strategy: Dark Mode by Default

By default, the tool does not apply anything. It generates code and runs `terraform plan` to show you what exists versus what the code describes. You review, you approve, you import. This prevents accidental changes during the migration itself.

### Terragrunt vs Terraform Modules

Both modes are supported. The default is Terraform with local modules — it is more portable and familiar to most teams. Terragrunt mode is available with `--mode terragrunt` for teams that need multi-environment promotion and DRY backend/provider configuration across many roots.

### AI Enrichment is Optional

The `--enrich` flag enables AI-assisted refinement. It is not required for the pipeline to produce valid, usable Terraform — the deterministic transformation alone gets you most of the way there. AI enrichment is the layer that makes the output look intentional rather than generated.

### No Stored Secrets

The bootstrap stage uses OIDC federated credentials on Managed Identities. There are no service principal passwords, no client secrets stored in pipeline variables, and no long-lived credentials of any kind.

---

## Prerequisites

- Azure CLI (`az`) logged in
- GitHub CLI (`gh`) logged in (for GitHub platform)
- `aztfexport` installed
- Go 1.22+

---

## Relationship to Existing Tools

| Tool | Role |
|---|---|
| [aztfexport](https://github.com/Azure/aztfexport) | Core export engine — introspects Azure and generates raw Terraform + state |
| Azure SDK for Go | Native bootstrap — provisions state storage, Managed Identities, OIDC, and RBAC |
| GitHub CLI (`gh`) | Repo creation, environment configuration, and variable management |
| [tflint](https://github.com/terraform-linters/tflint) | Linting pass applied after refine stage |
| [terraform-docs](https://github.com/terraform-docs/terraform-docs) | Documentation generation for module outputs |
| [Terragrunt](https://terragrunt.gruntwork.io/) | Optional DRY wrapper for multi-environment configurations |

`azlift` orchestrates these tools into a single pipeline and fills the gaps between them.

---

## Where This Fits in Platform Engineering

This tool is designed to address a specific, common scenario: an existing Azure environment with no IaC that needs to join a platform engineering practice. It is not a replacement for building new infrastructure code-first.

The recommended adoption path:

1. **Rescue** — use this tool to generate the initial IaC baseline
2. **Stabilize** — review the generated plan, correct deviations, merge into the pipeline
3. **Govern** — enforce that all future changes go through the pipeline; treat manual changes as drift
4. **Refactor** — incrementally improve the generated code toward your team's conventions and module library

---

## Status

This is a pilot project, developed as a practical demonstration of the azlift migration pattern. The goal is a working end-to-end prototype that covers the most common Azure resource types and both GitHub and Azure DevOps as CI/CD targets.

Contributions, feedback, and real-world test cases are welcome.
