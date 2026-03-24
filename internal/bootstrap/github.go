package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// RepoConfig holds parameters for GitHub repository creation.
type RepoConfig struct {
	// Org is the GitHub organisation.
	Org string
	// Name is the repository name.
	Name string
	// RepoDir is the local path of the repository to push.
	RepoDir string
	// Private creates a private repository (default: true).
	Private bool
}

// EnvVarConfig is a name/value pair for a GitHub Actions environment variable.
type EnvVarConfig struct {
	Name  string
	Value string
}

// GHEnvironmentConfig holds parameters for configuring GitHub environments.
type GHEnvironmentConfig struct {
	// Org is the GitHub organisation.
	Org string
	// Repo is the repository name.
	Repo string
	// Environments maps GitHub environment name → list of variables to set.
	Environments map[string][]EnvVarConfig
}

// CreateAndPushRepo creates the GitHub repository and pushes the local branch.
// Uses `gh repo create --source=. --push` which creates the remote, sets the
// origin remote, and pushes in one step.
func CreateAndPushRepo(ctx context.Context, cfg RepoConfig) error {
	visibility := "--public"
	if cfg.Private {
		visibility = "--private"
	}

	args := []string{
		"repo", "create",
		fmt.Sprintf("%s/%s", cfg.Org, cfg.Name),
		visibility,
		"--source=.",
		"--remote=origin",
		"--push",
	}
	if err := ghRun(ctx, cfg.RepoDir, args...); err != nil {
		return fmt.Errorf("gh repo create: %w", err)
	}
	return nil
}

// ConfigureEnvironments creates GitHub environments and sets the required
// Actions variables (AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID).
func ConfigureEnvironments(ctx context.Context, cfg GHEnvironmentConfig) error {
	for envName, vars := range cfg.Environments {
		// Create the environment via GitHub REST API.
		envEndpoint := fmt.Sprintf("repos/%s/%s/environments/%s", cfg.Org, cfg.Repo, envName)
		if err := ghRun(ctx, "", "api", envEndpoint, "--method=PUT", "--silent"); err != nil {
			return fmt.Errorf("creating environment %s: %w", envName, err)
		}

		// Set each variable.
		for _, v := range vars {
			body, err := json.Marshal(map[string]string{
				"name":  v.Name,
				"value": v.Value,
			})
			if err != nil {
				return err
			}
			varEndpoint := fmt.Sprintf("repos/%s/%s/environments/%s/variables", cfg.Org, cfg.Repo, envName)
			if err := ghRun(ctx, "",
				"api", varEndpoint,
				"--method=POST",
				"--silent",
				"--input=-",
				"--",
				"--body="+string(body),
			); err != nil {
				// 409 = variable already exists; update it instead.
				putEndpoint := fmt.Sprintf("repos/%s/%s/environments/%s/variables/%s", cfg.Org, cfg.Repo, envName, v.Name)
				if err2 := ghRun(ctx, "",
					"api", putEndpoint,
					"--method=PATCH",
					"--silent",
					"--field", fmt.Sprintf("name=%s", v.Name),
					"--field", fmt.Sprintf("value=%s", v.Value),
				); err2 != nil {
					return fmt.Errorf("setting variable %s on %s: %w", v.Name, envName, err2)
				}
			}
		}
	}
	return nil
}

// ghRun executes a gh CLI command in dir and returns a descriptive error on failure.
// Pass dir="" to run in the current working directory.
func ghRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // gh is a system tool
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh %v: %w\n%s", args, err, out.String())
	}
	return nil
}
