package main

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org-reporting",
		Short: "Org reporting build tools (DEV-PLAN-033)",
	}
	cmd.AddCommand(newBuildCmd())
	return cmd
}

func execute() {
	_ = newRootCmd().Execute()
}
