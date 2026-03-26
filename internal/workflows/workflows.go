// Package workflows provides embedded GitHub Actions workflow templates and
// renders them per deployment environment for the azlift pipeline.
package workflows

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*.tmpl
var embedded embed.FS

// Config controls which environments get workflow files and whether to use
// embedded templates or a custom directory.
type Config struct {
	// Environments is the list of deployment tiers (e.g. prod, staging, dev).
	Environments []string
	// Mode is "modules" (default) or "terragrunt". Selects the correct
	// workflow templates — terragrunt templates run commands from envs/<env>/
	// and check root.hcl instead of backend.tf.
	Mode string
	// CustomDir, when non-empty, reads .yml files from this directory instead
	// of the embedded templates. Files must be named plan.yml.tmpl and
	// apply.yml.tmpl and use the same {{.Environment}} template variable.
	CustomDir string
}

// Render returns a map of filename → rendered YAML content for all workflow
// files derived from cfg. Files are named plan-{env}.yml, apply-{env}.yml,
// and drift-{env}.yml.
func Render(cfg Config) (map[string][]byte, error) {
	planName, applyName, driftName := "plan.yml.tmpl", "apply.yml.tmpl", "drift.yml.tmpl"
	if cfg.Mode == "terragrunt" {
		planName, applyName, driftName = "plan-tg.yml.tmpl", "apply-tg.yml.tmpl", "drift-tg.yml.tmpl"
	}

	planTmpl, err := loadTemplate(cfg.CustomDir, planName)
	if err != nil {
		return nil, fmt.Errorf("loading plan template: %w", err)
	}
	applyTmpl, err := loadTemplate(cfg.CustomDir, applyName)
	if err != nil {
		return nil, fmt.Errorf("loading apply template: %w", err)
	}
	driftTmpl, err := loadTemplate(cfg.CustomDir, driftName)
	if err != nil {
		return nil, fmt.Errorf("loading drift template: %w", err)
	}

	files := make(map[string][]byte, len(cfg.Environments)*3)
	for _, env := range cfg.Environments {
		data := struct{ Environment string }{Environment: env}

		planBytes, err := renderTemplate(planTmpl, data)
		if err != nil {
			return nil, fmt.Errorf("rendering plan template for %s: %w", env, err)
		}
		files[fmt.Sprintf("plan-%s.yml", env)] = planBytes

		applyBytes, err := renderTemplate(applyTmpl, data)
		if err != nil {
			return nil, fmt.Errorf("rendering apply template for %s: %w", env, err)
		}
		files[fmt.Sprintf("apply-%s.yml", env)] = applyBytes

		driftBytes, err := renderTemplate(driftTmpl, data)
		if err != nil {
			return nil, fmt.Errorf("rendering drift template for %s: %w", env, err)
		}
		files[fmt.Sprintf("drift-%s.yml", env)] = driftBytes
	}

	return files, nil
}

// Write renders all workflow files and writes them to
// <repoDir>/.github/workflows/.
func Write(repoDir string, cfg Config) error {
	outDir := filepath.Join(repoDir, ".github", "workflows")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating workflows directory: %w", err)
	}

	files, err := Render(cfg)
	if err != nil {
		return err
	}

	for name, content := range files {
		dest := filepath.Join(outDir, name)
		if err := os.WriteFile(dest, content, 0o644); err != nil { //nolint:gosec // workflow files are not sensitive
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}
	return nil
}

// loadTemplate reads a template from CustomDir (if set) or from the embedded FS.
func loadTemplate(customDir, name string) (*template.Template, error) {
	var src []byte
	if customDir != "" {
		var err error
		src, err = os.ReadFile(filepath.Join(customDir, name)) //nolint:gosec
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		src, err = embedded.ReadFile("templates/" + name)
		if err != nil {
			return nil, err
		}
	}
	return template.New(name).Parse(string(src))
}

// renderTemplate executes tmpl with data and returns the result as bytes.
func renderTemplate(tmpl *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
