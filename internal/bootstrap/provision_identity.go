package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
)

const (
	// Well-known Azure built-in role definition IDs (identical in all tenants).
	roleIDReader      = "acdd72a7-3385-48ef-bd42-f606fba81ae7"
	roleIDContributor = "b24988ac-6180-42a0-ab88-20f7382dd24c"

	oidcIssuer   = "https://token.actions.githubusercontent.com"
	oidcAudience = "api://AzureADTokenExchange"
)

// IdentityProvisionConfig holds the parameters for provisioning Managed
// Identities with OIDC federation and RBAC assignments.
type IdentityProvisionConfig struct {
	// SubscriptionID is the Azure subscription for the MI resources.
	SubscriptionID string
	// ResourceGroup is the RG where MIs are created.
	ResourceGroup string
	// Location is the Azure region.
	Location string
	// RepoOrg is the GitHub organisation.
	RepoOrg string
	// RepoName is the GitHub repository name.
	RepoName string
	// Environments is the list of deployment tiers.
	Environments []string
	// RBACScopes are the full resource ID scopes for role assignments.
	// For same-tenant: one scope per managed resource group.
	// For cross-tenant: single subscription scope.
	RBACScopes []string
}

// ProvisionedIdentities is the result of a ProvisionIdentities call.
type ProvisionedIdentities struct {
	// Identities maps "<env>/<role>" → ManagedIdentity.
	Identities map[string]ManagedIdentity
}

// ProvisionIdentities creates plan+apply Managed Identities for each
// environment, configures OIDC federated credentials, and assigns the
// appropriate RBAC roles on all provided scopes.
func ProvisionIdentities(ctx context.Context, cred azcore.TokenCredential, cfg IdentityProvisionConfig) (ProvisionedIdentities, error) {
	result := ProvisionedIdentities{Identities: make(map[string]ManagedIdentity)}

	miClient, err := armmsi.NewUserAssignedIdentitiesClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return result, fmt.Errorf("creating MI client: %w", err)
	}
	fcClient, err := armmsi.NewFederatedIdentityCredentialsClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return result, fmt.Errorf("creating federated credential client: %w", err)
	}
	rbacClient, err := armauthorization.NewRoleAssignmentsClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return result, fmt.Errorf("creating RBAC client: %w", err)
	}

	for _, env := range cfg.Environments {
		for _, role := range []string{"plan", "apply"} {
			miName := MIName(cfg.RepoName, env, role)

			// 1. Create or update the Managed Identity.
			resp, err := miClient.CreateOrUpdate(ctx, cfg.ResourceGroup, miName, armmsi.Identity{
				Location: to.Ptr(cfg.Location),
			}, nil)
			if err != nil {
				return result, fmt.Errorf("creating MI %s: %w", miName, err)
			}
			mi := ManagedIdentity{
				Name:        miName,
				Environment: env,
				Role:        role,
				ClientID:    stringVal(resp.Properties.ClientID),
				ResourceID:  stringVal(resp.ID),
			}

			// 2. Configure OIDC federated credential.
			if err := configureFederatedCredential(ctx, fcClient, cfg, env, role, miName); err != nil {
				return result, err
			}

			// 3. Assign RBAC role on all scopes.
			roleDefID := buildRoleDefinitionID(cfg.SubscriptionID, rbacRoleForRole(role))
			for _, scope := range cfg.RBACScopes {
				if err := assignRole(ctx, rbacClient, scope, roleDefID, stringVal(resp.Properties.PrincipalID)); err != nil {
					return result, fmt.Errorf("assigning role for MI %s on %s: %w", miName, scope, err)
				}
			}

			result.Identities[fmt.Sprintf("%s/%s", env, role)] = mi
		}
	}

	return result, nil
}

// configureFederatedCredential creates or updates the OIDC federated credential
// for a Managed Identity.
func configureFederatedCredential(
	ctx context.Context,
	client *armmsi.FederatedIdentityCredentialsClient,
	cfg IdentityProvisionConfig,
	env, role, miName string,
) error {
	subject := oidcSubject(cfg.RepoOrg, cfg.RepoName, env, role)
	fcName := fmt.Sprintf("fc-%s-%s", env, role)

	_, err := client.CreateOrUpdate(ctx, cfg.ResourceGroup, miName, fcName,
		armmsi.FederatedIdentityCredential{
			Properties: &armmsi.FederatedIdentityCredentialProperties{
				Audiences: []*string{to.Ptr(oidcAudience)},
				Issuer:    to.Ptr(oidcIssuer),
				Subject:   to.Ptr(subject),
			},
		}, nil)
	if err != nil {
		return fmt.Errorf("configuring federated credential %s on %s: %w", fcName, miName, err)
	}
	return nil
}

// assignRole creates an RBAC role assignment. Existing assignments are silently
// ignored (idempotent).
func assignRole(ctx context.Context, client *armauthorization.RoleAssignmentsClient, scope, roleDefinitionID, principalID string) error {
	name := uuid.New().String()
	_, err := client.Create(ctx, scope, name, armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr(roleDefinitionID),
			PrincipalID:      to.Ptr(principalID),
			// PrincipalType must be set to avoid PrincipalNotFound errors caused
			// by AAD replication delay after MI creation.
			PrincipalType: to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
		},
	}, nil)
	if err != nil {
		// 409 Conflict = assignment already exists — safe to ignore.
		if isConflict(err) {
			return nil
		}
		return err
	}
	return nil
}

// buildRoleDefinitionID returns the full resource ID for a built-in role.
func buildRoleDefinitionID(subscriptionID, roleName string) string {
	var guid string
	switch roleName {
	case "Contributor":
		guid = roleIDContributor
	default:
		guid = roleIDReader
	}
	return fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", subscriptionID, guid)
}

// oidcSubject returns the GitHub Actions OIDC subject claim for a given role.
//
// When a workflow declares `environment:`, GitHub sets the subject to
// "repo:{org}/{repo}:environment:{name}" regardless of the trigger.
// Our workflows always use a named environment, so we must match that form.
func oidcSubject(org, repo, env, role string) string {
	// GitHub environment name mirrors what Write() puts in the workflow file.
	ghEnv := fmt.Sprintf("%s-iac-%s", env, role)
	return fmt.Sprintf("repo:%s/%s:environment:%s", org, repo, ghEnv)
}

// stringVal safely dereferences a *string, returning "" for nil.
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isConflict returns true if the error is an Azure 409 Conflict response,
// which means the role assignment already exists and can be safely ignored.
func isConflict(err error) bool {
	var respErr *azcore.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == 409
}
