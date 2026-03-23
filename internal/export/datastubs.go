package export

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UnsupportedType maps a resource type to the Terraform data source that
// should be generated as a stub.
type UnsupportedType struct {
	// ResourceType is the Azure resource type (lowercase).
	ResourceType string
	// DataSourceType is the Terraform data source type to generate.
	DataSourceType string
	// Reason explains why the type is not exportable.
	Reason string
}

// KnownUnsupportedTypes is the registry of resource types that aztfexport
// cannot export, along with the recommended data source substitute.
// Update this list as aztfexport adds support for new types.
var KnownUnsupportedTypes = []UnsupportedType{
	{
		ResourceType:   "microsoft.aad/domainservices",
		DataSourceType: "azurerm_active_directory_domain_service",
		Reason:         "Azure AD Domain Services cannot be managed by Terraform",
	},
	{
		ResourceType:   "microsoft.network/privatednszones/virtualnetworklinks",
		DataSourceType: "azurerm_private_dns_zone_virtual_network_link",
		Reason:         "Private DNS VNet links export is incomplete in aztfexport",
	},
	{
		ResourceType:   "microsoft.compute/galleries",
		DataSourceType: "azurerm_shared_image_gallery",
		Reason:         "Shared image galleries require manual state import",
	},
	{
		ResourceType:   "microsoft.containerservice/managedclusters/agentpools",
		DataSourceType: "azurerm_kubernetes_cluster_node_pool",
		Reason:         "AKS node pools are managed via the parent cluster resource",
	},
	{
		ResourceType:   "microsoft.network/bastionhosts",
		DataSourceType: "azurerm_bastion_host",
		Reason:         "Azure Bastion export requires manual state import in aztfexport",
	},
}

// unsupportedRegistry is a fast lookup map built from KnownUnsupportedTypes.
var unsupportedRegistry map[string]UnsupportedType

func init() {
	unsupportedRegistry = make(map[string]UnsupportedType, len(KnownUnsupportedTypes))
	for _, u := range KnownUnsupportedTypes {
		unsupportedRegistry[strings.ToLower(u.ResourceType)] = u
	}
}

// IsUnsupported returns the UnsupportedType entry and true if the resource
// type is known to be unsupported by aztfexport.
func IsUnsupported(resourceType string) (UnsupportedType, bool) {
	u, ok := unsupportedRegistry[strings.ToLower(resourceType)]
	return u, ok
}

// ResourceRef holds the minimum information needed to generate a data stub.
type ResourceRef struct {
	ID            string
	Name          string
	Type          string
	ResourceGroup string
}

// GenerateDataStub writes a Terraform data source stub HCL file for a
// resource that aztfexport cannot export. The file is placed in outputDir.
func GenerateDataStub(ref ResourceRef, outputDir string) (string, error) {
	u, ok := IsUnsupported(ref.Type)
	if !ok {
		return "", fmt.Errorf("resource type %q is not in the unsupported registry", ref.Type)
	}

	localName := sanitiseName(ref.Name)
	hcl := buildDataStubHCL(u, ref, localName)

	filename := fmt.Sprintf("data_%s_%s.tf", sanitiseName(u.DataSourceType), localName)
	path := filepath.Join(outputDir, filename)

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(hcl), 0o600); err != nil {
		return "", fmt.Errorf("writing data stub: %w", err)
	}
	return path, nil
}

func buildDataStubHCL(u UnsupportedType, ref ResourceRef, localName string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# TODO(azlift): %s\n", u.Reason)
	fmt.Fprintf(&sb, "# Source resource: %s\n", ref.ID)
	fmt.Fprint(&sb, "# Update the name argument to match your environment.\n\n")
	fmt.Fprintf(&sb, "data %q %q {\n", u.DataSourceType, localName)
	fmt.Fprintf(&sb, "  name                = %q\n", ref.Name)
	fmt.Fprintf(&sb, "  resource_group_name = %q\n", ref.ResourceGroup)
	fmt.Fprint(&sb, "}\n")
	return sb.String()
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// sanitiseName converts an Azure resource name to a valid Terraform identifier.
func sanitiseName(name string) string {
	lower := strings.ToLower(name)
	clean := nonAlphaNum.ReplaceAllString(lower, "_")
	return strings.Trim(clean, "_")
}
