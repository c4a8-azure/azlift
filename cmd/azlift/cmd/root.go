package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	subscription string
	verbose      bool
)

var rootCmd = &cobra.Command{
	Use:   "azlift",
	Short: "Turn ClickOps Azure resources into production-ready Terraform/Terragrunt",
	Long: `azlift orchestrates aztfexport and az-bootstrap into a single pipeline that
converts portal-created Azure resources into structured, CI/CD-ready Terraform
or Terragrunt code — fully wired into a Git-based GitOps workflow.

Pipeline stages:
  scan       Build resource inventory and dependency graph
  export     Export resources via aztfexport
  refine     Transform raw HCL into structured Terraform or Terragrunt
  bootstrap  Provision state storage, Managed Identities, and Git CI/CD
  run        Run the full pipeline end-to-end (scan → export → refine → bootstrap)`,
	SilenceUsage: true,
}

// Execute is the entry point called from main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&subscription, "subscription", "", "Azure subscription ID")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(
		newScanCmd(),
		newExportCmd(),
		newRefineCmd(),
		newBootstrapCmd(),
		newRunCmd(),
	)
}

// notImplemented is a shared RunE body for stub commands.
func notImplemented(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(os.Stderr, "[azlift] %s: not yet implemented\n", cmd.CommandPath())
	return nil
}
