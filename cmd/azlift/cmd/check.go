package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/tools"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify all required external tools are present and meet minimum versions",
		Long: `Run the external tool detector and print a status table. Exits 0 when all
required tools pass, non-zero if any required tool is missing or too old.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results, err := tools.CheckRequired(context.Background(), tools.DefaultTools)
			for _, r := range results {
				if r.Err != nil {
					fmt.Fprintf(os.Stderr, "  FAIL  %v\n", r.Err)
				} else {
					fmt.Printf("  OK    %-16s %s\n", r.Tool.Name, r.Version)
				}
			}
			return err
		},
	}
}
