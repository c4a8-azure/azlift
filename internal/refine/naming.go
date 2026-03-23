package refine

import (
	"strings"
)

// azureRegions maps common Azure region short codes (as they appear in resource
// names) to their canonical location identifiers. Only the most frequent
// abbreviations are listed; unrecognised suffixes are left as-is.
var azureRegions = map[string]string{
	"westeu":             "westeurope",
	"westeurope":         "westeurope",
	"eastus":             "eastus",
	"eastus2":            "eastus2",
	"westus":             "westus",
	"westus2":            "westus2",
	"westus3":            "westus3",
	"northeu":            "northeurope",
	"northeurope":        "northeurope",
	"centralus":          "centralus",
	"uksouth":            "uksouth",
	"ukwest":             "ukwest",
	"australiaeast":      "australiaeast",
	"australiasoutheast": "australiasoutheast",
	"southeastasia":      "southeastasia",
	"eastasia":           "eastasia",
	"japaneast":          "japaneast",
	"japanwest":          "japanwest",
	"canadacentral":      "canadacentral",
	"canadaeast":         "canadaeast",
	"brazilsouth":        "brazilsouth",
	"southafricanorth":   "southafricanorth",
	"uaenorth":           "uaenorth",
}

// knownEnvs are common environment segment values found in Azure resource names.
var knownEnvs = map[string]bool{
	"prod":    true,
	"prd":     true,
	"dev":     true,
	"test":    true,
	"tst":     true,
	"stg":     true,
	"staging": true,
	"uat":     true,
	"qa":      true,
	"sandbox": true,
	"sbx":     true,
	"shared":  true,
	"mgmt":    true,
	"hub":     true,
}

// resourceTypePrefixes maps the well-known Azure naming prefix to the resource
// type it identifies. Used only to recognise that a segment is a type prefix
// and should be skipped when extracting workload / prefix segments.
var resourceTypePrefixes = map[string]bool{
	"rg":     true,
	"vnet":   true,
	"snet":   true,
	"kv":     true,
	"st":     true,
	"vm":     true,
	"vmss":   true,
	"nic":    true,
	"pip":    true,
	"nsg":    true,
	"rt":     true,
	"pe":     true,
	"pep":    true,
	"agw":    true,
	"lb":     true,
	"adf":    true,
	"aks":    true,
	"acr":    true,
	"app":    true,
	"func":   true,
	"plan":   true,
	"sql":    true,
	"sqldb":  true,
	"cosmos": true,
	"ci":     true,
	"apim":   true,
	"log":    true,
	"law":    true,
	"aa":     true,
	"id":     true,
}

// ParsedName holds the components decoded from an Azure resource name.
// Fields are empty strings when a component could not be detected.
type ParsedName struct {
	// Environment is the lifecycle tier (prod, dev, staging …).
	Environment string
	// Region is the canonical Azure location string (westeurope, eastus …).
	Region string
	// Prefix collects workload / project segments that remain after stripping
	// the type prefix, environment, and region tokens.
	Prefix string
}

// ParseName attempts to decode an Azure resource name that follows the
// common CAF pattern: <type>-<env>-<workload…>-<region>[-<suffix>]
//
// Parsing is best-effort: unknown segments are absorbed into Prefix.
// Returns a zero-value ParsedName when name is empty or has < 2 segments.
func ParseName(name string) ParsedName {
	parts := strings.Split(strings.ToLower(name), "-")
	if len(parts) < 2 {
		return ParsedName{}
	}

	var result ParsedName
	var prefixParts []string

	// Walk parts, consuming known tokens.
	for i, p := range parts {
		switch {
		case i == 0 && resourceTypePrefixes[p]:
			// Type prefix — skip.
		case result.Environment == "" && knownEnvs[p]:
			result.Environment = p
		case result.Region == "" && azureRegions[p] != "":
			result.Region = azureRegions[p]
		default:
			// Ignore pure numeric suffix segments (001, 01, …).
			if !isNumeric(p) {
				prefixParts = append(prefixParts, p)
			}
		}
	}

	result.Prefix = strings.Join(prefixParts, "-")
	return result
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// AnalyseNames scans resource blocks across files and calls ParseName on the
// "name" attribute of each block. It returns the set of unique non-empty
// environments and regions found.
func AnalyseNames(files []*ParsedFile) (environments, regions []string) {
	envSeen := map[string]bool{}
	regSeen := map[string]bool{}

	for _, pf := range files {
		for _, block := range Blocks(pf, "resource") {
			attr := block.Body().GetAttribute("name")
			if attr == nil {
				continue
			}
			val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
			if !isStringLiteral(val) {
				continue
			}
			name := unquote(val)
			parsed := ParseName(name)
			if parsed.Environment != "" && !envSeen[parsed.Environment] {
				envSeen[parsed.Environment] = true
				environments = append(environments, parsed.Environment)
			}
			if parsed.Region != "" && !regSeen[parsed.Region] {
				regSeen[parsed.Region] = true
				regions = append(regions, parsed.Region)
			}
		}
	}
	return environments, regions
}
