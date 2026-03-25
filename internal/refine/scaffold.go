package refine

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// BackendConfig carries the values needed to write an azurerm backend block.
// Fields that are empty strings become placeholder comments in the output.
type BackendConfig struct {
	// ResourceGroupName of the Terraform state storage resource group.
	ResourceGroupName string
	// StorageAccountName holds the state storage account.
	StorageAccountName string
	// ContainerName is the blob container used for state files.
	ContainerName string
	// StateKey is the blob path, typically "<rg-name>/terraform.tfstate".
	StateKey string
}

// DefaultBackendConfig returns a BackendConfig with sensible placeholder values
// that can be replaced after BOOTSTRAP runs.
func DefaultBackendConfig(resourceGroup string) BackendConfig {
	return BackendConfig{
		ResourceGroupName:  "rg-tfstate", // placeholder
		StorageAccountName: "sttfstate",  // placeholder
		ContainerName:      "tfstate",
		StateKey:           fmt.Sprintf("%s/terraform.tfstate", resourceGroup),
	}
}

// GenerateBackend writes backend.tf into outputDir. If cfg fields are empty,
// placeholder strings are used so the file is always valid HCL.
func GenerateBackend(outputDir string, cfg BackendConfig) (*ParsedFile, error) {
	pf := NewFile(filepath.Join(outputDir, "backend.tf"))
	body := pf.File.Body()

	tfBlock := body.AppendNewBlock("terraform", nil)
	beBlock := tfBlock.Body().AppendNewBlock("backend", []string{"azurerm"})
	be := beBlock.Body()

	be.SetAttributeValue("resource_group_name", cty.StringVal(orPlaceholder(cfg.ResourceGroupName, "<resource_group_name>")))
	be.SetAttributeValue("storage_account_name", cty.StringVal(orPlaceholder(cfg.StorageAccountName, "<storage_account_name>")))
	be.SetAttributeValue("container_name", cty.StringVal(orPlaceholder(cfg.ContainerName, "tfstate")))
	be.SetAttributeValue("key", cty.StringVal(orPlaceholder(cfg.StateKey, "terraform.tfstate")))

	return pf, nil
}

// ProviderPin describes a required_providers entry.
type ProviderPin struct {
	Source  string
	Version string
}

// DefaultProviderPins returns the standard azurerm + azapi pins used by azlift.
var DefaultProviderPins = []ProviderPin{
	{Source: "hashicorp/azurerm", Version: "~> 4.0"},
	{Source: "azure/azapi", Version: "~> 2.0"},
}

// DefaultMinTerraformVersion is the required_version injected when aztfexport
// omits it and no override is provided via Options.MinTerraformVersion.
const DefaultMinTerraformVersion = ">= 1.10"

// ExtractTerraformBlock finds the terraform {} block from the parsed input
// files, strips any embedded backend {} sub-block (which lives in backend.tf),
// and writes the result as terraform.tf in outputDir.
// minVersion is injected as required_version when the input block omits it;
// pass "" to use DefaultMinTerraformVersion.
// Returns nil when no terraform block is present in the input — the caller
// should fall back to GenerateVersions in that case.
func ExtractTerraformBlock(outputDir string, files []*ParsedFile, minVersion string) (*ParsedFile, error) {
	if minVersion == "" {
		minVersion = DefaultMinTerraformVersion
	}
	for _, pf := range files {
		for _, block := range pf.File.Body().Blocks() {
			if block.Type() != "terraform" {
				continue
			}
			// Re-parse a mutable clone of the block.
			src := block.BuildTokens(nil).Bytes()
			f, diags := hclwrite.ParseConfig(append(src, '\n'), "<terraform>", initPos)
			if diags.HasErrors() {
				continue
			}
			// Strip backend sub-block — it lives in backend.tf.
			// Also inject required_version if aztfexport omitted it (tflint requires it).
			for _, outer := range f.Body().Blocks() {
				if outer.Type() != "terraform" {
					continue
				}
				for _, sub := range outer.Body().Blocks() {
					if sub.Type() == "backend" {
						outer.Body().RemoveBlock(sub)
					}
				}
				if outer.Body().GetAttribute("required_version") == nil {
					outer.Body().SetAttributeValue("required_version", cty.StringVal(minVersion))
				}
			}
			out := NewFile(filepath.Join(outputDir, "terraform.tf"))
			out.File = f
			return out, nil
		}
	}
	return nil, nil
}

// GenerateVersions writes terraform.tf into outputDir with a terraform block
// containing required_version and required_providers. Used as a fallback when
// the input files contain no terraform block (e.g. when refining without a
// prior aztfexport run).
func GenerateVersions(outputDir string, minTerraformVersion string, pins []ProviderPin) (*ParsedFile, error) {
	if minTerraformVersion == "" {
		minTerraformVersion = DefaultMinTerraformVersion
	}
	if len(pins) == 0 {
		pins = DefaultProviderPins
	}

	pf := NewFile(filepath.Join(outputDir, "terraform.tf"))
	body := pf.File.Body()

	tfBlock := body.AppendNewBlock("terraform", nil)
	tfBody := tfBlock.Body()

	tfBody.SetAttributeValue("required_version", cty.StringVal(minTerraformVersion))

	rpBlock := tfBody.AppendNewBlock("required_providers", nil)
	rpBody := rpBlock.Body()

	for _, pin := range pins {
		// Each provider is an object: { source = "...", version = "..." }
		rpBody.SetAttributeRaw(
			providerAlias(pin.Source),
			tokensForProviderObject(pin.Source, pin.Version),
		)
	}

	return pf, nil
}

// GenerateProvider writes a minimal provider block for azurerm.
// storage_use_azuread = true prevents the provider from calling
// listKeys on storage accounts during plan/apply — the plan MI only
// has Reader, not the Contributor needed for that action.
func GenerateProvider(outputDir string) (*ParsedFile, error) {
	pf := NewFile(filepath.Join(outputDir, "providers.tf"))
	body := pf.File.Body()

	p := body.AppendNewBlock("provider", []string{"azurerm"})
	pb := p.Body()
	pb.SetAttributeValue("storage_use_azuread", cty.True)
	pb.AppendNewBlock("features", nil)

	return pf, nil
}

// providerAlias derives the local alias from "org/name" → "name".
func providerAlias(source string) string {
	parts := splitLast(source, "/")
	return parts[len(parts)-1]
}

// splitLast splits s by sep and returns all parts.
func splitLast(s, sep string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// tokensForProviderObject builds the RHS token sequence for:
//
//	{ source = "hashicorp/azurerm", version = "~> 4.0" }
func tokensForProviderObject(source, version string) hclwrite.Tokens {
	// Build a throwaway file with the object literal, then extract the tokens.
	src := fmt.Sprintf(`x = { source = %q, version = %q }`, source, version)
	f, diags := hclwrite.ParseConfig([]byte(src+"\n"), "<gen>", initPos)
	if diags.HasErrors() {
		// Fallback: plain string value.
		return hclwrite.TokensForValue(cty.StringVal(source))
	}
	attrs := f.Body().Attributes()
	if a, ok := attrs["x"]; ok {
		return a.Expr().BuildTokens(nil)
	}
	return hclwrite.TokensForValue(cty.StringVal(source))
}

func orPlaceholder(val, placeholder string) string {
	if val == "" {
		return placeholder
	}
	return val
}
