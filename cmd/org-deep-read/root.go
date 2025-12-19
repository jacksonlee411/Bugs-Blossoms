package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "org-deep-read",
		Short:         "Org deep read build/refresh tool (DEV-PLAN-029)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newClosureCmd())
	return cmd
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
