package main

import "github.com/spf13/cobra"

func newBenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run benchmarks and emit JSON reports",
	}
	cmd.AddCommand(newBenchTreeCmd())
	return cmd
}
