package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type closureOutput struct {
	Command    string `json:"command"`
	DurationMS int64  `json:"duration_ms"`
	Result     any    `json:"result"`
}

func newClosureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "closure",
		Short: "Temporal closure build/activate/prune",
	}
	cmd.AddCommand(newClosureBuildCmd())
	cmd.AddCommand(newClosureActivateCmd())
	cmd.AddCommand(newClosurePruneCmd())
	return cmd
}

func newClosureBuildCmd() *cobra.Command {
	var (
		tenantID      string
		hierarchyType string
		apply         bool
		requestID     string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build and (optionally) activate temporal closure deep-read table",
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			if requestID == "" {
				requestID = uuid.NewString()
			}

			pool, err := connectDB(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx := composables.WithPool(cmd.Context(), pool)
			repo := persistence.NewOrgRepository()

			start := time.Now()
			res, err := repo.BuildDeepReadClosure(ctx, tid, hierarchyType, apply, requestID)
			if err != nil {
				return err
			}
			out := closureOutput{
				Command:    "closure build",
				DurationMS: time.Since(start).Milliseconds(),
				Result:     res,
			}
			return writeJSON(out)
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&hierarchyType, "hierarchy", "OrgUnit", "Hierarchy type")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply changes (default dry-run)")
	cmd.Flags().StringVar(&requestID, "request-id", "", "Source request id (optional)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func newClosureActivateCmd() *cobra.Command {
	var (
		tenantID      string
		hierarchyType string
		buildID       string
	)

	cmd := &cobra.Command{
		Use:   "activate",
		Short: "Activate a ready closure build (rollback by activating a previous build)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			bid, err := uuid.Parse(buildID)
			if err != nil {
				return fmt.Errorf("invalid --build-id: %w", err)
			}

			pool, err := connectDB(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx := composables.WithPool(cmd.Context(), pool)
			repo := persistence.NewOrgRepository()

			start := time.Now()
			prev, err := repo.ActivateDeepReadClosureBuild(ctx, tid, hierarchyType, bid)
			if err != nil {
				return err
			}
			out := closureOutput{
				Command:    "closure activate",
				DurationMS: time.Since(start).Milliseconds(),
				Result: map[string]any{
					"tenant_id":      tid.String(),
					"hierarchy_type": hierarchyType,
					"build_id":       bid.String(),
					"previous_build_id": func() string {
						if prev == uuid.Nil {
							return ""
						}
						return prev.String()
					}(),
				},
			}
			return writeJSON(out)
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&hierarchyType, "hierarchy", "OrgUnit", "Hierarchy type")
	cmd.Flags().StringVar(&buildID, "build-id", "", "Build UUID (required)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("build-id")
	return cmd
}

func newClosurePruneCmd() *cobra.Command {
	var (
		tenantID      string
		hierarchyType string
		keep          int
	)

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune non-active closure builds (keeps N newest including active)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			if keep <= 0 {
				keep = 1
			}

			pool, err := connectDB(cmd.Context())
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx := composables.WithPool(cmd.Context(), pool)
			repo := persistence.NewOrgRepository()

			start := time.Now()
			res, err := repo.PruneDeepReadClosureBuilds(ctx, tid, hierarchyType, keep)
			if err != nil {
				return err
			}
			out := closureOutput{
				Command:    "closure prune",
				DurationMS: time.Since(start).Milliseconds(),
				Result:     res,
			}
			return writeJSON(out)
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&hierarchyType, "hierarchy", "OrgUnit", "Hierarchy type")
	cmd.Flags().IntVar(&keep, "keep", 2, "How many builds to keep (>=1)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}
