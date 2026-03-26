package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
)

// TfStateUploadConfig holds the parameters for uploading terraform.tfstate.
type TfStateUploadConfig struct {
	// SubscriptionID is used to fetch the storage account key via ARM.
	SubscriptionID string
	// ResourceGroupName is the RG containing the storage account.
	ResourceGroupName string
	// StorageAccountName is the globally-unique storage account name.
	StorageAccountName string
	// ContainerName is the blob container (default: "tfstate").
	ContainerName string
	// BlobKey is the blob path (e.g. "infra-prod.tfstate").
	BlobKey string
	// LocalPath is the path to the local terraform.tfstate file.
	LocalPath string
}

// UploadTfState uploads the local terraform.tfstate produced by aztfexport to
// the remote Azure blob container. This only runs in same-tenant mode; for
// cross-tenant deployments the state file is abandoned (source resource IDs
// are meaningless in the target tenant).
//
// Auth: uses the storage account key fetched via ARM (avoids requiring
// Storage Blob Data Contributor on the caller's identity).
func UploadTfState(ctx context.Context, cred azcore.TokenCredential, cfg TfStateUploadConfig) error {
	container := cfg.ContainerName
	if container == "" {
		container = "tfstate"
	}

	data, err := os.ReadFile(cfg.LocalPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("reading tfstate file %s: %w", cfg.LocalPath, err)
	}

	// Fetch the storage account key via ARM — this uses management-plane auth
	// which the caller already holds. This sidesteps needing Storage Blob Data
	// Contributor on the data plane.
	key, err := fetchStorageAccountKey(ctx, cred, cfg.SubscriptionID, cfg.ResourceGroupName, cfg.StorageAccountName)
	if err != nil {
		return fmt.Errorf("fetching storage account key: %w", err)
	}

	sharedKey, err := azblob.NewSharedKeyCredential(cfg.StorageAccountName, key)
	if err != nil {
		return fmt.Errorf("building shared key credential: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.StorageAccountName)
	client, err := azblob.NewClientWithSharedKeyCredential(serviceURL, sharedKey, nil)
	if err != nil {
		return fmt.Errorf("creating blob client: %w", err)
	}

	_, err = client.UploadBuffer(ctx, container, cfg.BlobKey, data, &azblob.UploadBufferOptions{
		Metadata: map[string]*string{
			"uploadedBy": ptr("azlift"),
		},
	})
	if err != nil && !bloberror.HasCode(err, bloberror.BlobAlreadyExists) {
		return fmt.Errorf("uploading tfstate to %s/%s/%s: %w", cfg.StorageAccountName, container, cfg.BlobKey, err)
	}
	return nil
}

// fetchStorageAccountKey retrieves the first storage account access key via ARM.
func fetchStorageAccountKey(ctx context.Context, cred azcore.TokenCredential, subscriptionID, resourceGroup, accountName string) (string, error) {
	client, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.ListKeys(ctx, resourceGroup, accountName, nil)
	if err != nil {
		return "", fmt.Errorf("listing keys for %s: %w", accountName, err)
	}
	if len(resp.Keys) == 0 || resp.Keys[0].Value == nil {
		return "", fmt.Errorf("no keys returned for storage account %s", accountName)
	}
	return *resp.Keys[0].Value, nil
}

func ptr(s string) *string { return &s }
