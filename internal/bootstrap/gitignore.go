package bootstrap

import (
	"os"
	"path/filepath"
)

// terraformGitignore is the .gitignore written into every bootstrapped
// repository. State files and plan files may contain resource attribute
// values — including passwords, connection strings, and access keys — in
// plain text. They must never be committed to version control.
const terraformGitignore = `# ============================================================
# Terraform — files that must NEVER be committed
# ============================================================

# State files contain plain-text resource attributes, which often include
# secrets (connection strings, passwords, access keys, SAS tokens, …).
terraform.tfstate
terraform.tfstate.backup
*.tfstate
*.tfstate.*

# Plan files serialise the full proposed resource graph including all
# attribute values. Treat them like state files.
*.tfplan
*.tfplan.json

# Terraform working directory — contains cached provider binaries and
# may contain tokens written by provider authentication helpers.
.terraform/

# Crash logs should not be tracked.
crash.log
crash.*.log

# Override files are intended for local developer use only.
override.tf
override.tf.json
*_override.tf
*_override.tf.json

# tfvars files that may contain secrets.
# terraform.tfvars with only non-sensitive defaults is safe to commit;
# add it here if it contains any secret values.
*.auto.tfvars
*.auto.tfvars.json

# ============================================================
# OS / editor noise
# ============================================================
.DS_Store
Thumbs.db
.idea/
.vscode/
*.swp
*.swo
`

// neverCommitFiles is the set of filenames that copyDirContents must
// never copy into the repository directory, regardless of .gitignore.
// This is a belt-and-suspenders guard on top of the .gitignore.
var neverCommitFiles = map[string]bool{
	"terraform.tfstate":        true,
	"terraform.tfstate.backup": true,
}

// WriteGitignore writes the standard Terraform .gitignore into repoDir.
// It overwrites any existing .gitignore so the protection is always current.
func WriteGitignore(repoDir string) error {
	return os.WriteFile(
		filepath.Join(repoDir, ".gitignore"),
		[]byte(terraformGitignore),
		0o600,
	)
}
