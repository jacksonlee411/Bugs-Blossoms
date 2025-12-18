package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type rollbackOptions struct {
	tenantID     uuid.UUID
	manifestPath string
	since        *time.Time
	apply        bool
	yes          bool
}

func newRollbackCmd() *cobra.Command {
	var opts rollbackOptions
	var tenant string
	var since string

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback Org seed import by manifest or since timestamp",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if stringsTrim(since) != "" {
				t, err := parseTimeField(since)
				if err != nil {
					return withCode(exitUsage, fmt.Errorf("invalid --since: %w", err))
				}
				opts.since = &t
			}
			return runRollback(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&opts.manifestPath, "manifest", "", "Path to import_manifest_*.json")
	cmd.Flags().StringVar(&since, "since", "", "Rollback seed data since this time (RFC3339)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "Apply rollback (default is dry-run)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Confirm destructive rollback")
	_ = cmd.MarkFlagRequired("tenant")

	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(stringsTrim(tenant))
		if err != nil {
			return withCode(exitUsage, fmt.Errorf("invalid --tenant: %w", err))
		}
		opts.tenantID = id
		return nil
	}

	return cmd
}

func runRollback(ctx context.Context, opts rollbackOptions) error {
	if opts.tenantID == uuid.Nil {
		return withCode(exitUsage, fmt.Errorf("--tenant is required"))
	}

	if (stringsTrim(opts.manifestPath) == "") == (opts.since == nil) {
		return withCode(exitUsage, fmt.Errorf("exactly one of --manifest or --since is required"))
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer pool.Close()

	if err := ensureTenantExists(ctx, pool, opts.tenantID); err != nil {
		return err
	}

	if stringsTrim(opts.manifestPath) != "" {
		manifest, err := readManifest(opts.manifestPath)
		if err != nil {
			return withCode(exitValidation, err)
		}
		if manifest.TenantID != opts.tenantID {
			return withCode(exitValidation, fmt.Errorf("manifest tenant_id mismatch: %s", manifest.TenantID))
		}
		if !opts.apply {
			return printRollbackSummary("dry_run", "manifest", manifest, nil)
		}
		if !opts.yes {
			return withCode(exitSafetyNet, fmt.Errorf("refusing to rollback without --yes"))
		}
		if err := rollbackByManifest(ctx, pool, opts.tenantID, manifest); err != nil {
			return err
		}
		return printRollbackSummary("applied", "manifest", manifest, nil)
	}

	if opts.since != nil {
		if !opts.apply {
			return printRollbackSummary("dry_run", "since", nil, opts.since)
		}
		if !opts.yes {
			return withCode(exitSafetyNet, fmt.Errorf("refusing to rollback without --yes"))
		}
		if err := rollbackBySince(ctx, pool, opts.tenantID, *opts.since); err != nil {
			return err
		}
		return printRollbackSummary("applied", "since", nil, opts.since)
	}

	return withCode(exitUsage, fmt.Errorf("unreachable"))
}

func readManifest(path string) (*importManifestV1, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var m importManifestV1
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("unsupported manifest version: %d", m.Version)
	}
	if m.Backend != "db" || m.Mode != "seed" {
		return nil, fmt.Errorf("unsupported manifest backend/mode: %s/%s", m.Backend, m.Mode)
	}
	return &m, nil
}

func rollbackByManifest(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, m *importManifestV1) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return withCode(exitDB, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTenantID(ctx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return withCode(exitDB, err)
	}

	if err := deleteByIDs(txCtx, tx, "org_assignments", m.Inserted.OrgAssignments); err != nil {
		return err
	}
	if err := deleteByIDs(txCtx, tx, "org_positions", m.Inserted.OrgPositions); err != nil {
		return err
	}
	if err := deleteByIDs(txCtx, tx, "org_edges", m.Inserted.OrgEdges); err != nil {
		return err
	}
	if err := deleteByIDs(txCtx, tx, "org_node_slices", m.Inserted.OrgNodeSlices); err != nil {
		return err
	}
	if err := deleteByIDs(txCtx, tx, "org_nodes", m.Inserted.OrgNodes); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return withCode(exitDB, fmt.Errorf("commit tx: %w", err))
	}
	return nil
}

func deleteByIDs(ctx context.Context, tx pgx.Tx, table string, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return withCode(exitDB, err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE tenant_id=$1 AND id = ANY($2)`, table), tenantID, ids)
	if err != nil {
		return withCode(exitDBWrite, fmt.Errorf("delete %s: %w", table, err))
	}
	return nil
}

func rollbackBySince(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, since time.Time) error {
	var older int
	if err := pool.QueryRow(ctx, `
		SELECT 1 FROM org_nodes
		WHERE tenant_id=$1 AND created_at < $2
		LIMIT 1
	`, tenantID, since).Scan(&older); err == nil {
		return withCode(exitSafetyNet, fmt.Errorf("refusing rollback --since: org_nodes has rows created before %s", since.Format(time.RFC3339)))
	} else if err != pgx.ErrNoRows {
		return withCode(exitDB, fmt.Errorf("rollback precheck: %w", err))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return withCode(exitDB, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTenantID(ctx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return withCode(exitDB, err)
	}

	del := func(table string) error {
		_, err := tx.Exec(txCtx, fmt.Sprintf(`DELETE FROM %s WHERE tenant_id=$1 AND created_at >= $2`, table), tenantID, since)
		if err != nil {
			return withCode(exitDBWrite, fmt.Errorf("delete %s: %w", table, err))
		}
		return nil
	}

	if err := del("org_assignments"); err != nil {
		return err
	}
	if err := del("org_positions"); err != nil {
		return err
	}
	if err := del("org_edges"); err != nil {
		return err
	}
	if err := del("org_node_slices"); err != nil {
		return err
	}
	if err := del("org_nodes"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return withCode(exitDB, fmt.Errorf("commit tx: %w", err))
	}
	return nil
}

func printRollbackSummary(status, mode string, manifest *importManifestV1, since *time.Time) error {
	type rollbackSummary struct {
		Status   string  `json:"status"`
		Mode     string  `json:"mode"`
		RunID    *string `json:"run_id,omitempty"`
		TenantID *string `json:"tenant_id,omitempty"`
		Since    *string `json:"since,omitempty"`
		Counts   *struct {
			OrgNodes       int `json:"org_nodes"`
			OrgNodeSlices  int `json:"org_node_slices"`
			OrgEdges       int `json:"org_edges"`
			OrgPositions   int `json:"org_positions"`
			OrgAssignments int `json:"org_assignments"`
		} `json:"counts,omitempty"`
	}

	var s rollbackSummary
	s.Status = status
	s.Mode = mode
	if manifest != nil {
		runID := manifest.RunID.String()
		tenantID := manifest.TenantID.String()
		s.RunID = &runID
		s.TenantID = &tenantID
		s.Counts = &struct {
			OrgNodes       int `json:"org_nodes"`
			OrgNodeSlices  int `json:"org_node_slices"`
			OrgEdges       int `json:"org_edges"`
			OrgPositions   int `json:"org_positions"`
			OrgAssignments int `json:"org_assignments"`
		}{
			OrgNodes:       len(manifest.Inserted.OrgNodes),
			OrgNodeSlices:  len(manifest.Inserted.OrgNodeSlices),
			OrgEdges:       len(manifest.Inserted.OrgEdges),
			OrgPositions:   len(manifest.Inserted.OrgPositions),
			OrgAssignments: len(manifest.Inserted.OrgAssignments),
		}
	}
	if since != nil {
		v := since.UTC().Format(time.RFC3339)
		s.Since = &v
	}
	return writeJSONLine(s)
}
