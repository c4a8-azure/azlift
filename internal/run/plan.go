package run

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// tfPlanRunner runs terraform plan in a directory and captures its output.
type tfPlanRunner struct{}

// Plan runs `terraform init` then `terraform plan` in dir.
// It returns the combined plan output and any error.
func (r *tfPlanRunner) Plan(ctx context.Context, log *slog.Logger, dir string) (string, error) {
	// terraform init
	log.Info("[PLAN] running terraform init", "dir", dir)
	initOut, err := runTF(ctx, dir, "init", "-input=false")
	if err != nil {
		return initOut, fmt.Errorf("terraform init: %w", err)
	}

	// terraform plan
	log.Info("[PLAN] running terraform plan", "dir", dir)
	planOut, err := runTF(ctx, dir, "plan", "-input=false")
	if err != nil {
		return planOut, fmt.Errorf("terraform plan: %w", err)
	}

	return strings.TrimSpace(planOut), nil
}

func runTF(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "terraform", args...) //nolint:gosec // args are controlled by us
	cmd.Dir = dir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}
