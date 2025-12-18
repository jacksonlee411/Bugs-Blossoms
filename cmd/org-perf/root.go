package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org-perf",
		Short: "Org performance tools (DEV-PLAN-027)",
	}
	cmd.AddCommand(newDatasetCmd())
	cmd.AddCommand(newBenchCmd())
	return cmd
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
