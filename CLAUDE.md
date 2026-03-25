# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`azlift` is a Go CLI tool that orchestrates [aztfexport](https://github.com/Azure/aztfexport) into a pipeline that converts portal-created ("ClickOps") Azure resources into production-ready Terraform or Terragrunt code, fully wired into a Git-based CI/CD setup. The bootstrap stage provisions state storage, Managed Identities with OIDC, and the GitHub repository entirely via native Azure SDK and `gh` CLI calls — no external PowerShell modules required.

## Language: Go — Final Decision

**azlift is written in Go. This is a closed decision — do not propose Python, Rust, or any other language.**

The reasoning, so this does not need to be relitigated:

1. **HCL manipulation is the core of REFINE** — `hashicorp/hcl` and `hclwrite` are the canonical Go libraries, maintained by HashiCorp and used by Terraform itself. They support parsing *and* generating HCL with full AST fidelity. No other language has an equivalent. Python's `python-hcl2` is parse-only; Rust has no mature HCL library.
2. **Single binary distribution** — Go compiles to a self-contained binary with no runtime dependencies. This is the standard for platform engineering CLI tools (Terraform, aztfexport, tflint, helm, kubectl are all Go binaries).
3. **Ecosystem alignment** — aztfexport (the tool azlift wraps most heavily) is Go. Same language means potential programmatic integration, shared types, and familiar idioms.

## Prerequisites / External Dependencies

The tool wraps and depends on these external tools:
- `az` (Azure CLI) — authenticated session required
- `aztfexport` — core export engine
- `gh` (GitHub CLI) — when targeting GitHub
- `tflint` — linting pass after refine stage
- `terraform-docs` — documentation generation
- Go 1.22+

## Pipeline Stages

The four-stage pipeline is the core abstraction. Every major feature maps to one of these:

1. **SCAN** — Azure Resource Graph API queries to build resource inventory and cross-RG dependency graph; determines module/state boundaries before any export
2. **EXPORT** — Wraps `aztfexport` per logical boundary (typically per resource group) with retry logic, exclusion lists, and unsupported-resource mapping to `data` sources
3. **REFINE** — Multi-step HCL transformation pipeline:
   - Variable extraction (repeated literals → `variable`/`locals`)
   - Semantic analysis (decode naming conventions → structured variables)
   - Resource grouping (flat `main.tf` → `networking.tf`, `compute.tf`, etc.)
   - Backend and provider generation
   - Optional: `--mode terragrunt` generates layered Terragrunt structure instead
   - Optional: `--enrich` AI pass for lifecycle rules, security flags, descriptions
4. **BOOTSTRAP** — Native Go pipeline: provisions state storage and Managed Identities with OIDC federated credentials via Azure SDK, creates the GitHub repository via `gh` CLI, writes GitHub Actions workflows per environment, and uploads the exported `terraform.tfstate` to remote storage

## CLI Entry Points

```bash
# Full pipeline (single command)
azlift run --subscription <id> --resource-group <rg> --repo-name <name> [--mode terragrunt] [--enrich] [--platform github|ado]

# Individual stages
azlift scan --subscription <id>
azlift export --resource-group <rg> --output-dir ./raw
azlift refine --input-dir ./raw --mode modules|terragrunt [--enrich]
azlift bootstrap --input-dir ./refined --repo-name <name> --environments dev,staging,prod --platform github|ado

# Emergency/incident mode — generate + plan without bootstrapping
azlift run ... --no-bootstrap --dry-run
```

## Implementation Order (GitHub Issues)

Work is tracked as epics with sub-issues. The mandatory sequencing:

```
#1 Foundation  →  #2 SCAN  →  #3 EXPORT  →  #4 REFINE (core)
                                                     │
                              ┌──────────────────────┼──────────────────────┐
                              ▼                      ▼                      ▼
                        #5 REFINE             #6 AI Enrichment        #7 BOOTSTRAP
                        (Terragrunt)          (--enrich)
                              │                      │                      │
                              └──────────────────────┴──────────────────────┘
                                                     ▼
                                               #8 RUN (full pipeline)
```

- **#1 Foundation** must be complete before any stage work begins — CLI skeleton, `PipelineContext`, logger, external tool detector, CI.
- **#2 SCAN** before **#3 EXPORT** — scan output defines the boundaries that export operates on.
- **#3 EXPORT** before **#4 REFINE** — refine consumes export output.
- **#5**, **#6**, **#7** can proceed in parallel once **#4** is done — they are independent extensions of the refine output.
- **#8 RUN** is last — it orchestrates all stages end-to-end and requires every other epic to be complete.

## Git Workflow

- **One branch per issue** — when starting work on any GitHub issue, create a branch named `issue-<number>-<short-slug>` (e.g. `issue-9-go-module-init`).
- **Commit often** — commit at each logical checkpoint within the issue (e.g. after scaffolding files, after tests pass, after wiring into CLI). Small commits are easier to review and revert.
- **PR per issue** — open a pull request from the issue branch to `main` when the issue is complete; reference the issue in the PR body so GitHub closes it on merge.

## Key Design Constraints

- **Dark mode by default** — the tool never applies changes; it generates code and runs `terraform plan` only. Users explicitly import after reviewing.
- **State boundaries per resource group** — aligns with Azure RBAC boundaries and blast radius; cross-root references via `data` sources and remote state outputs.
- **No stored secrets** — bootstrap uses OIDC federated credentials on Managed Identities throughout; no service principal passwords or client secrets anywhere.
- **Deterministic core, optional AI** — the refine stage must produce valid, usable Terraform without `--enrich`; AI enrichment is a quality layer on top, not a requirement.
- **Terraform modules is the default** — `--mode terragrunt` is opt-in for teams needing multi-environment DRY promotion.
