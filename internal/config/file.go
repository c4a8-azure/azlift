package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPaths is the ordered list of locations searched when no
// explicit config file is provided.
var DefaultConfigPaths = []string{
	".azlift.yaml",
	".azlift.yml",
	"azlift.yaml",
	"azlift.yml",
}

// Load reads a PipelineContext from a YAML config file. If path is empty the
// function searches DefaultConfigPaths; if none exist it returns an empty
// context with defaults applied (not an error).
func Load(path string) (*PipelineContext, error) {
	resolved, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	ctx := &PipelineContext{}

	if resolved == "" {
		// No config file found — start from defaults.
		ctx.Defaults()
		return ctx, nil
	}

	data, err := os.ReadFile(resolved) //nolint:gosec // path is user-supplied config file, not untrusted input
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", resolved, err)
	}

	if err := yaml.Unmarshal(data, ctx); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", resolved, err)
	}

	ctx.Defaults()
	return ctx, nil
}

// MergeFlags overwrites fields in ctx with any non-zero values from flags.
// This is called after Load so that CLI flags always take precedence over
// the config file.
func MergeFlags(ctx *PipelineContext, flags Flags) {
	if flags.SubscriptionID != "" {
		ctx.SubscriptionID = flags.SubscriptionID
	}
	if flags.ResourceGroup != "" {
		ctx.ResourceGroup = flags.ResourceGroup
	}
	if flags.RepoName != "" {
		ctx.RepoName = flags.RepoName
	}
	if flags.Mode != "" {
		ctx.Mode = Mode(flags.Mode)
	}
	if flags.Platform != "" {
		ctx.Platform = Platform(flags.Platform)
	}
	if len(flags.Environments) > 0 {
		ctx.Environments = flags.Environments
	}
	if flags.ExportOutputDir != "" {
		ctx.ExportOutputDir = flags.ExportOutputDir
	}
	if flags.RefineOutputDir != "" {
		ctx.RefineOutputDir = flags.RefineOutputDir
	}
	if flags.Enrich {
		ctx.Enrich = true
	}
	if flags.NoBootstrap {
		ctx.NoBootstrap = true
	}
	if flags.DryRun {
		ctx.DryRun = true
	}
	if flags.Verbose {
		ctx.Verbose = true
	}
}

// Flags holds the raw values parsed from cobra flags before they are merged
// into PipelineContext. String fields use "" as "not set".
type Flags struct {
	SubscriptionID  string
	ResourceGroup   string
	RepoName        string
	Mode            string
	Platform        string
	Environments    []string
	ExportOutputDir string
	RefineOutputDir string
	Enrich          bool
	NoBootstrap     bool
	DryRun          bool
	Verbose         bool
}

// Validate returns an error if the context is missing fields required for
// the named stage. Stage names match the cobra subcommand names.
func Validate(ctx *PipelineContext, stage string) error {
	if ctx.SubscriptionID == "" && stage != "refine" && stage != "bootstrap" {
		return errors.New("--subscription is required")
	}
	switch stage {
	case "export", "run":
		if ctx.ResourceGroup == "" {
			return fmt.Errorf("--%s requires --resource-group", stage)
		}
	case "bootstrap":
		if ctx.RepoName == "" {
			return errors.New("--bootstrap requires --repo-name")
		}
	}
	if ctx.Mode != ModeModules && ctx.Mode != ModeTerragrunt {
		return fmt.Errorf("invalid --mode %q: must be modules or terragrunt", ctx.Mode)
	}
	if ctx.Platform != PlatformGitHub && ctx.Platform != PlatformADO {
		return fmt.Errorf("invalid --platform %q: must be github or ado", ctx.Platform)
	}
	return nil
}

func resolvePath(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config file not found: %s", explicit)
		}
		return explicit, nil
	}
	for _, p := range DefaultConfigPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", nil
}
