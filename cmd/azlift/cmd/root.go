package cmd

import (
	"fmt"
	"os"

	"github.com/c4a8-azure/azlift/internal/logger"
	"github.com/spf13/cobra"
)

var (
	subscription string
	verbose      bool
	quiet        bool
	outputFormat string
)

// Log is the root-level logger, available to all subcommands. Subcommands
// should call Log.WithStage to get a stage-labelled logger.
var Log *logger.Logger

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
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		return initLogger()
	},
}

// Execute is the entry point called from main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&subscription, "subscription", "", "Azure subscription ID")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug-level output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output except errors")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "text", "Log format: text or json")

	rootCmd.AddCommand(
		newScanCmd(),
		newExportCmd(),
		newRefineCmd(),
		newBootstrapCmd(),
		newRunCmd(),
		newCheckCmd(),
	)

	// Initialise a default logger before PersistentPreRunE fires (e.g. for
	// flag parse errors that surface before any command runs).
	Log = logger.New(logger.StageRoot, logger.Options{})
}

func initLogger() error {
	switch outputFormat {
	case "text", "json":
	default:
		return fmt.Errorf("invalid --output %q: must be text or json", outputFormat)
	}
	Log = logger.New(logger.StageRoot, logger.Options{
		Verbose: verbose,
		Format:  logger.Format(outputFormat),
		Writer:  os.Stderr,
	})
	return nil
}

// notImplemented is a shared RunE body for stub commands.
func notImplemented(cmd *cobra.Command, _ []string) error {
	Log.Info("not yet implemented", "command", cmd.CommandPath())
	return nil
}
