package main

import "github.com/spf13/cobra"

func newQualityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quality",
		Short: "Org data quality checks & fixes (DEV-PLAN-031)",
	}
	cmd.AddCommand(newQualityCheckCmd())
	cmd.AddCommand(newQualityPlanCmd())
	cmd.AddCommand(newQualityApplyCmd())
	cmd.AddCommand(newQualityRollbackCmd())
	return cmd
}
