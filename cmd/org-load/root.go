package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "org-load",
		Short:         "Org load testing tool (DEV-PLAN-034)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newSmokeCmd())
	cmd.AddCommand(newRunCmd())
	return cmd
}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
