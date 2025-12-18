package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "org-data",
		Short:         "Org data import/export/rollback tool (DEV-PLAN-023)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newRollbackCmd())
	return cmd
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		code := exitCode(err)
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(code)
	}
}
