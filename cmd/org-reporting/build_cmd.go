package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/modules/org/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type buildOutput struct {
	Command    string `json:"command"`
	DurationMS int64  `json:"duration_ms"`
	Result     any    `json:"result"`
}

func newBuildCmd() *cobra.Command {
	var (
		tenantID      string
		hierarchyType string
		asOfDate      string
		apply         bool
		requestID     string
		includeSG     bool
		includeLinks  bool
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build org_reporting_nodes from active snapshot build",
		RunE: func(cmd *cobra.Command, args []string) error {
			tid, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("invalid --tenant: %w", err)
			}
			d, err := parseDateUTC(asOfDate)
			if err != nil {
				return err
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
			res, err := repo.BuildOrgReportingNodes(ctx, tid, hierarchyType, d, includeSG, includeLinks, apply, requestID)
			if err != nil {
				return err
			}

			out := buildOutput{
				Command:    "reporting build",
				DurationMS: time.Since(start).Milliseconds(),
				Result:     res,
			}
			return writeJSON(out)
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&hierarchyType, "hierarchy", "OrgUnit", "Hierarchy type")
	cmd.Flags().StringVar(&asOfDate, "as-of-date", time.Now().UTC().Format("2006-01-02"), "As-of date (UTC, YYYY-MM-DD)")
	cmd.Flags().BoolVar(&includeSG, "include-security-groups", false, "Populate security_group_keys (requires DEV-PLAN-032 tables)")
	cmd.Flags().BoolVar(&includeLinks, "include-links", false, "Populate links (requires DEV-PLAN-032 tables)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply changes (default dry-run)")
	cmd.Flags().StringVar(&requestID, "request-id", "", "Source request id (optional)")
	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}
