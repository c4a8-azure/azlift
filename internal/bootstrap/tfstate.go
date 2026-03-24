package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// TfStateUploadConfig holds the parameters for uploading terraform.tfstate.
type TfStateUploadConfig struct {
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
func UploadTfState(ctx context.Context, cred azcore.TokenCredential, cfg TfStateUploadConfig) error {
	container := cfg.ContainerName
	if container == "" {
		container = "tfstate"
	}

	data, err := os.ReadFile(cfg.LocalPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("reading tfstate file %s: %w", cfg.LocalPath, err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.StorageAccountName)
	client, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return fmt.Errorf("creating blob client: %w", err)
	}

	_, err = client.UploadBuffer(ctx, container, cfg.BlobKey, data, &azblob.UploadBufferOptions{
		Metadata: map[string]*string{
			"uploadedBy": ptr("azlift"),
		},
	})
	if err != nil {
		return fmt.Errorf("uploading tfstate to %s/%s/%s: %w", cfg.StorageAccountName, container, cfg.BlobKey, err)
	}
	return nil
}

func ptr(s string) *string { return &s }
