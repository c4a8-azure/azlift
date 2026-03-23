// Package config defines PipelineContext — the single source of truth threaded
// through every azlift pipeline stage — together with the shared config types
// and file-loading logic.
package config

// Mode controls the output format produced by the refine stage.
type Mode string

const (
	ModeModules    Mode = "modules"
	ModeTerragrunt Mode = "terragrunt"
)

// Platform selects the CI/CD target for the bootstrap stage.
type Platform string

const (
	PlatformGitHub Platform = "github"
	PlatformADO    Platform = "ado"
)

// PipelineContext is the canonical state object passed between all four
// pipeline stages (scan → export → refine → bootstrap). It is populated once
// from a config file and CLI flags (flags win), then passed by pointer so each
// stage can annotate it with its outputs.
type PipelineContext struct {
	// Azure targeting
	SubscriptionID string `yaml:"subscription_id"`
	ResourceGroup  string `yaml:"resource_group"`

	// Directories — populated during execution; can be seeded from config
	ScanOutputDir   string `yaml:"scan_output_dir"`
	ExportOutputDir string `yaml:"export_output_dir"`
	RefineOutputDir string `yaml:"refine_output_dir"`

	// Behaviour flags
	Mode        Mode     `yaml:"mode"`
	Platform    Platform `yaml:"platform"`
	Enrich      bool     `yaml:"enrich"`
	NoBootstrap bool     `yaml:"no_bootstrap"`
	DryRun      bool     `yaml:"dry_run"`
	Verbose     bool     `yaml:"verbose"`

	// Repository
	RepoName     string   `yaml:"repo_name"`
	Environments []string `yaml:"environments"`

	// Scan-stage outputs (populated by stage 1, read by stage 2)
	ResourceGroups []ResourceGroupSummary `yaml:"-"`
}

// ResourceGroupSummary is produced by the scan stage and consumed by export.
type ResourceGroupSummary struct {
	Name          string
	ResourceCount int
	ResourceTypes []string
	DependsOn     []string // names of other resource groups this one references
}

// Defaults applies zero-value defaults so callers do not need to guard
// against empty strings for optional fields.
func (c *PipelineContext) Defaults() {
	if c.Mode == "" {
		c.Mode = ModeModules
	}
	if c.Platform == "" {
		c.Platform = PlatformGitHub
	}
	if len(c.Environments) == 0 {
		c.Environments = []string{"dev", "staging", "prod"}
	}
	if c.ExportOutputDir == "" {
		c.ExportOutputDir = "./raw"
	}
	if c.RefineOutputDir == "" {
		c.RefineOutputDir = "./refined"
	}
}
