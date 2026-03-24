// Package bootstrap implements Stage 4 of the azlift pipeline: provisioning
// state storage, Managed Identities with OIDC federated credentials, the Git
// repository, and CI/CD workflows — all natively in Go without external
// PowerShell dependencies.
//
// For same-tenant deployments the pipeline provisions Azure resources directly
// via the Azure SDK and uploads the aztfexport tfstate to the new blob container.
//
// For cross-tenant deployments a bootstrap/ Terraform module is generated in the
// output repository for the operator to apply manually in the target tenant.
package bootstrap
