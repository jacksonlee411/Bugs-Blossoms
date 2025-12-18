package main

import "github.com/spf13/cobra"

func newDatasetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dataset",
		Short: "Generate/import deterministic datasets",
	}
	cmd.AddCommand(newDatasetApplyCmd())
	return cmd
}
