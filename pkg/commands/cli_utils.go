package commands

import (
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/modules"
	"github.com/iota-uz/iota-sdk/modules/superadmin"
	"github.com/iota-uz/iota-sdk/pkg/application"
)

// NewUtilityCommands creates all utility commands (check_tr_keys, seed, seed_superadmin)
func NewUtilityCommands() []*cobra.Command {
	return []*cobra.Command{
		newCheckTrKeysCmd(),
		newCheckTrUsageCmd(),
		newSeedCmd(),
		newSeed061Cmd(),
		newSeedSuperadminCmd(),
	}
}

func newCheckTrKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check_tr_keys",
		Short: "Check translation key consistency across all locales",
		Long:  `Validates that all translation keys are present across all configured locales and reports any missing translations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mods := append([]application.Module(nil), modules.BuiltInModules...)
			mods = append(mods, superadmin.NewModule(nil))
			return CheckTrKeys(nil, mods...)
		},
	}
}

func newCheckTrUsageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check_tr_usage",
		Short: "Check translation usages for missing keys in allowed locales",
		Long:  `Scans .go/.templ files for translation usages and ensures every referenced key exists in runtime-allowed locales (en/zh).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mods := append([]application.Module(nil), modules.BuiltInModules...)
			mods = append(mods, superadmin.NewModule(nil))
			return CheckTrUsage(nil, mods...)
		},
	}
}

func newSeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed the main database with initial data",
		Long:  `Populates the main database with initial seed data including default tenant, users, permissions, and configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SeedDatabase(modules.BuiltInModules...)
		},
	}
}

func newSeed061Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed_061",
		Short: "Seed DEV-PLAN-061 sample dataset",
		Long:  `Creates an Org-Position-Person bridge sample dataset (20 employees) following DEV-PLAN-061: persons, org nodes, assignments (auto positions), and personnel events (hire/transfer/termination).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SeedDevPlan061(modules.BuiltInModules...)
		},
	}
}

func newSeedSuperadminCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed_superadmin",
		Short: "Seed the database with a superadmin user",
		Long:  `Creates a superadmin user with full permissions for accessing the Super Admin dashboard.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SeedSuperadmin(modules.BuiltInModules...)
		},
	}
}
