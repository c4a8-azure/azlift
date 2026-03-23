package tools

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Tool describes an external binary that azlift depends on.
type Tool struct {
	// Name is the executable name looked up on PATH.
	Name string
	// VersionArgs are the arguments passed to obtain version output.
	VersionArgs []string
	// VersionRe extracts the semantic version string from the output.
	VersionRe *regexp.Regexp
	// MinVersion is the minimum acceptable [major, minor, patch].
	MinVersion [3]int
	// Required means a missing tool is a hard failure; optional tools emit
	// a warning only.
	Required bool
	// InstallURL is shown in the error message when the tool is absent.
	InstallURL string
	// Stage is the pipeline stage that needs this tool (informational).
	Stage string
}

// CheckResult is the outcome of checking a single tool.
type CheckResult struct {
	Tool    *Tool
	Found   bool
	Version string // empty when not found
	Err     error  // non-nil means the check failed (missing or too old)
}

// String returns a single-line summary suitable for log output.
func (r CheckResult) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%-16s FAIL  %v", r.Tool.Name, r.Err)
	}
	return fmt.Sprintf("%-16s OK    %s", r.Tool.Name, r.Version)
}

// Required tools with minimum versions and install URLs.
var DefaultTools = []*Tool{
	{
		Name:        "az",
		VersionArgs: []string{"version", "--output", "tsv", "--query", "\"azure-cli\""},
		VersionRe:   regexp.MustCompile(`(\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{2, 50, 0},
		Required:    true,
		InstallURL:  "https://docs.microsoft.com/cli/azure/install-azure-cli",
		Stage:       "scan/export/bootstrap",
	},
	{
		Name:        "aztfexport",
		VersionArgs: []string{"version"},
		VersionRe:   regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{0, 14, 0},
		Required:    true,
		InstallURL:  "https://github.com/Azure/aztfexport/releases",
		Stage:       "export",
	},
	{
		Name:        "pwsh",
		VersionArgs: []string{"--version"},
		VersionRe:   regexp.MustCompile(`(\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{7, 0, 0},
		Required:    true,
		InstallURL:  "https://github.com/PowerShell/PowerShell/releases",
		Stage:       "bootstrap",
	},
	{
		Name:        "gh",
		VersionArgs: []string{"version"},
		VersionRe:   regexp.MustCompile(`gh version (\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{2, 0, 0},
		Required:    true,
		InstallURL:  "https://cli.github.com",
		Stage:       "bootstrap",
	},
	{
		Name:        "terraform",
		VersionArgs: []string{"version", "-json"},
		VersionRe:   regexp.MustCompile(`(\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{1, 5, 0},
		Required:    true,
		InstallURL:  "https://developer.hashicorp.com/terraform/install",
		Stage:       "refine/bootstrap",
	},
	{
		Name:        "tflint",
		VersionArgs: []string{"--version"},
		VersionRe:   regexp.MustCompile(`TFLint version (\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{0, 50, 0},
		Required:    false,
		InstallURL:  "https://github.com/terraform-linters/tflint",
		Stage:       "refine",
	},
	{
		Name:        "terraform-docs",
		VersionArgs: []string{"version"},
		VersionRe:   regexp.MustCompile(`v(\d+\.\d+\.\d+)`),
		MinVersion:  [3]int{0, 16, 0},
		Required:    false,
		InstallURL:  "https://terraform-docs.io/user-guide/installation/",
		Stage:       "refine",
	},
}

// CheckAll runs all tool checks in parallel and returns results in the same
// order as the input slice. ctx cancellation stops in-flight execs but does
// not short-circuit the result collection — all results are always returned.
func CheckAll(ctx context.Context, tools []*Tool) []CheckResult {
	results := make([]CheckResult, len(tools))
	var wg sync.WaitGroup

	for i, t := range tools {
		wg.Add(1)
		go func(idx int, tool *Tool) {
			defer wg.Done()
			results[idx] = checkOne(ctx, tool)
		}(i, t)
	}

	wg.Wait()
	return results
}

// CheckRequired runs CheckAll and returns an error listing every hard failure
// (Required=true tools that are missing or too old). Optional tool failures
// are included in results but do not contribute to the returned error.
func CheckRequired(ctx context.Context, tools []*Tool) ([]CheckResult, error) {
	results := CheckAll(ctx, tools)

	var failures []string
	for _, r := range results {
		if r.Err != nil && r.Tool.Required {
			failures = append(failures, r.Err.Error())
		}
	}

	if len(failures) > 0 {
		return results, fmt.Errorf("missing or incompatible required tools:\n  %s",
			strings.Join(failures, "\n  "))
	}
	return results, nil
}

// checkOne checks a single tool and returns its CheckResult.
func checkOne(ctx context.Context, t *Tool) CheckResult {
	path, err := exec.LookPath(t.Name)
	if err != nil {
		msg := fmt.Sprintf("%s: not found on PATH — install from %s", t.Name, t.InstallURL)
		return CheckResult{Tool: t, Found: false, Err: fmt.Errorf("%s", msg)}
	}
	_ = path

	cmd := exec.CommandContext(ctx, t.Name, t.VersionArgs...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return CheckResult{
			Tool:  t,
			Found: true,
			Err:   fmt.Errorf("%s: version check failed: %w", t.Name, err),
		}
	}

	version := extractVersion(string(out), t.VersionRe)
	if version == "" {
		// Tool exists but we couldn't parse its version — treat as OK with
		// an unknown version rather than failing (defensive).
		return CheckResult{Tool: t, Found: true, Version: "unknown"}
	}

	if err := checkMinVersion(t.Name, version, t.MinVersion, t.InstallURL); err != nil {
		return CheckResult{Tool: t, Found: true, Version: version, Err: err}
	}

	return CheckResult{Tool: t, Found: true, Version: version}
}

// extractVersion returns the first version string matched by re, or "".
func extractVersion(output string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// checkMinVersion returns an error if version < min.
func checkMinVersion(name, version string, min [3]int, installURL string) error {
	parts := strings.SplitN(version, ".", 3)
	nums := [3]int{}
	for i, p := range parts {
		if i >= 3 {
			break
		}
		// Strip any trailing non-numeric suffix (e.g. "1-beta").
		fields := strings.FieldsFunc(p, func(r rune) bool { return r < '0' || r > '9' })
		if len(fields) == 0 {
			continue // unparseable component — leave as zero
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue // unparseable — leave as zero, skip version check
		}
		if i < len(nums) {
			nums[i] = n //nolint:gosec // G602 false positive: SplitN(...,3) and [3]int are both len 3; bounds checked above
		}
	}

	for i := range 3 {
		if nums[i] > min[i] {
			return nil // definitely newer
		}
		if nums[i] < min[i] {
			return fmt.Errorf("%s %s is below minimum %d.%d.%d — upgrade from %s",
				name, version, min[0], min[1], min[2], installURL)
		}
	}
	return nil // exactly equal
}
