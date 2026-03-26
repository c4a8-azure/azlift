// Package gitrepo provides lightweight wrappers around the git CLI for the
// local repository operations needed by the azlift pipeline.
package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Init runs `git init` in dir, creating a new repository.
func Init(ctx context.Context, dir string) error {
	return run(ctx, dir, "init", "-b", "main")
}

// ConfigUser sets the local git user name and email so commits do not require a
// global git config (useful in CI and temp directories).
func ConfigUser(ctx context.Context, dir, name, email string) error {
	if err := run(ctx, dir, "config", "user.name", name); err != nil {
		return err
	}
	return run(ctx, dir, "config", "user.email", email)
}

// Add stages the given paths. Pass "." to stage everything.
func Add(ctx context.Context, dir string, paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	return run(ctx, dir, args...)
}

// Commit creates a commit with the given message. Returns an error if there is
// nothing to commit.
func Commit(ctx context.Context, dir, message string) error {
	return run(ctx, dir, "commit", "--message", message)
}

// AddRemote adds a remote with the given name and URL.
func AddRemote(ctx context.Context, dir, name, url string) error {
	return run(ctx, dir, "remote", "add", name, url)
}

// CreateBranch creates and checks out a new local branch.
func CreateBranch(ctx context.Context, dir, name string) error {
	return run(ctx, dir, "checkout", "-b", name)
}

// Push pushes branch to remote. Uses --set-upstream on first push.
func Push(ctx context.Context, dir, remote, branch string) error {
	return run(ctx, dir, "push", "--set-upstream", remote, branch)
}

// Clone clones repoURL into dir.
func Clone(ctx context.Context, repoURL, dir string) error {
	return run(ctx, "", "clone", repoURL, dir)
}

// run shells out to git with args inside dir and returns a descriptive error
// that includes any captured stderr/stdout on failure.
func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // git is a system tool
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out.String())
	}
	return nil
}
