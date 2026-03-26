package bootstrap

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// ProvisionStateStorage creates the resource group, storage account, and blob
// container required for Terraform remote state.
//
// All operations are idempotent — running against already-existing resources
// is safe and returns no error.
func ProvisionStateStorage(ctx context.Context, cred azcore.TokenCredential, cfg StateStorageConfig) error {
	if err := ensureResourceGroup(ctx, cred, cfg.SubscriptionID, cfg.ResourceGroupName, cfg.Location); err != nil {
		return fmt.Errorf("ensuring state resource group: %w", err)
	}
	if err := ensureStorageAccount(ctx, cred, cfg.SubscriptionID, cfg.ResourceGroupName, cfg.StorageAccountName, cfg.Location); err != nil {
		return fmt.Errorf("ensuring storage account: %w", err)
	}
	if err := ensureBlobContainer(ctx, cred, cfg.SubscriptionID, cfg.ResourceGroupName, cfg.StorageAccountName, cfg.ContainerName); err != nil {
		return fmt.Errorf("ensuring blob container: %w", err)
	}
	return nil
}

func ensureResourceGroup(ctx context.Context, cred azcore.TokenCredential, subscriptionID, rgName, location string) error {
	client, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return err
	}
	_, err = client.CreateOrUpdate(ctx, rgName, armresources.ResourceGroup{
		Location: to.Ptr(location),
	}, nil)
	return err
}

func ensureStorageAccount(ctx context.Context, cred azcore.TokenCredential, subscriptionID, rg, accountName, location string) error {
	client, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return err
	}
	poller, err := client.BeginCreate(ctx, rg, accountName, armstorage.AccountCreateParameters{
		Location: to.Ptr(location),
		Kind:     to.Ptr(armstorage.KindStorageV2),
		SKU:      &armstorage.SKU{Name: to.Ptr(armstorage.SKUNameStandardLRS)},
		Properties: &armstorage.AccountPropertiesCreateParameters{
			MinimumTLSVersion:      to.Ptr(armstorage.MinimumTLSVersionTLS12),
			AllowBlobPublicAccess:  to.Ptr(false),
			EnableHTTPSTrafficOnly: to.Ptr(true),
		},
	}, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	return err
}

func ensureBlobContainer(ctx context.Context, cred azcore.TokenCredential, subscriptionID, rg, accountName, containerName string) error {
	client, err := armstorage.NewBlobContainersClient(subscriptionID, cred, nil)
	if err != nil {
		return err
	}
	_, err = client.Create(ctx, rg, accountName, containerName, armstorage.BlobContainer{
		ContainerProperties: &armstorage.ContainerProperties{
			PublicAccess: to.Ptr(armstorage.PublicAccessNone),
		},
	}, nil)
	return err
}
