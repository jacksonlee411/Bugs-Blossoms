package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	tenantID  uuid.UUID
	outputDir string
	asOf      *time.Time
}

func newExportCmd() *cobra.Command {
	var opts exportOptions
	var tenant string
	var asOf string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Org data from DB into CSV files",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if stringsTrim(asOf) != "" {
				t, err := parseTimeField(asOf)
				if err != nil {
					return withCode(exitUsage, fmt.Errorf("invalid --as-of: %w", err))
				}
				opts.asOf = &t
			}
			return runExport(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&opts.outputDir, "output", "", "Output directory (required)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "As-of time (YYYY-MM-DD or RFC3339)")
	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("output")

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

func runExport(ctx context.Context, opts exportOptions) error {
	if opts.tenantID == uuid.Nil {
		return withCode(exitUsage, fmt.Errorf("--tenant is required"))
	}
	if stringsTrim(opts.outputDir) == "" {
		return withCode(exitUsage, fmt.Errorf("--output is required"))
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer pool.Close()

	if err := ensureTenantExists(ctx, pool, opts.tenantID); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.outputDir, 0o755); err != nil {
		return withCode(exitDB, err)
	}

	if err := exportNodes(ctx, pool, opts); err != nil {
		return err
	}
	if err := exportPositions(ctx, pool, opts); err != nil {
		return err
	}
	if err := exportAssignments(ctx, pool, opts); err != nil {
		return err
	}

	type exportSummary struct {
		Status   string `json:"status"`
		TenantID string `json:"tenant_id"`
	}
	return writeJSONLine(exportSummary{
		Status:   "exported",
		TenantID: opts.tenantID.String(),
	})
}

func ensureTenantExists(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) error {
	var dummy int
	if err := pool.QueryRow(ctx, `SELECT 1 FROM tenants WHERE id=$1`, tenantID).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return withCode(exitValidation, fmt.Errorf("unknown tenant: %s", tenantID))
		}
		return withCode(exitDB, fmt.Errorf("check tenant existence: %w", err))
	}
	return nil
}

func exportNodes(ctx context.Context, pool *pgxpool.Pool, opts exportOptions) error {
	path := filepath.Join(opts.outputDir, "nodes.csv")
	f, err := os.Create(path)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	header := []string{
		"code", "type", "name", "i18n_names", "status", "legal_entity_id", "company_code", "location_id",
		"display_order", "parent_code", "manager_user_id", "manager_email", "effective_date", "end_date",
	}
	if err := w.Write(header); err != nil {
		return withCode(exitDB, err)
	}

	var rows pgx.Rows
	if opts.asOf != nil {
		rows, err = pool.Query(ctx, `
			SELECT
				n.code,
				n.type,
				s.name,
				s.i18n_names::text,
				s.status,
				COALESCE(s.legal_entity_id::text, ''),
				COALESCE(s.company_code, ''),
				COALESCE(s.location_id::text, ''),
				s.display_order,
				COALESCE(pn.code, ''),
				COALESCE(s.manager_user_id::text, ''),
				'' AS manager_email,
				s.effective_date,
				s.end_date
			FROM org_nodes n
			JOIN org_node_slices s
			  ON s.tenant_id = n.tenant_id
			 AND s.org_node_id = n.id
			 AND s.effective_date <= $2 AND s.end_date > $2
			LEFT JOIN org_edges e
			  ON e.tenant_id = n.tenant_id
			 AND e.child_node_id = n.id
			 AND e.effective_date <= $2 AND e.end_date > $2
			LEFT JOIN org_nodes pn
			  ON pn.tenant_id = n.tenant_id
			 AND pn.id = e.parent_node_id
			WHERE n.tenant_id = $1
			ORDER BY n.code ASC
		`, opts.tenantID, *opts.asOf)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT
				n.code,
				n.type,
				s.name,
				s.i18n_names::text,
				s.status,
				COALESCE(s.legal_entity_id::text, ''),
				COALESCE(s.company_code, ''),
				COALESCE(s.location_id::text, ''),
				s.display_order,
				COALESCE(pn.code, ''),
				COALESCE(s.manager_user_id::text, ''),
				'' AS manager_email,
				s.effective_date,
				s.end_date
			FROM org_node_slices s
			JOIN org_nodes n
			  ON n.tenant_id = s.tenant_id
			 AND n.id = s.org_node_id
			LEFT JOIN org_edges e
			  ON e.tenant_id = s.tenant_id
			 AND e.child_node_id = s.org_node_id
			 AND e.effective_date <= s.effective_date AND e.end_date > s.effective_date
			LEFT JOIN org_nodes pn
			  ON pn.tenant_id = s.tenant_id
			 AND pn.id = e.parent_node_id
			WHERE s.tenant_id = $1
			ORDER BY n.code ASC, s.effective_date ASC
		`, opts.tenantID)
	}
	if err != nil {
		return withCode(exitDB, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			code, typ, name, i18n, status           string
			legalEntityID, companyCode, locationID  string
			displayOrder                            int
			parentCode, managerUserID, managerEmail string
			effectiveDate, endDate                  time.Time
		)
		if err := rows.Scan(
			&code, &typ, &name, &i18n, &status,
			&legalEntityID, &companyCode, &locationID,
			&displayOrder, &parentCode, &managerUserID, &managerEmail,
			&effectiveDate, &endDate,
		); err != nil {
			return withCode(exitDB, err)
		}
		rec := []string{
			code, typ, name, i18n, status,
			legalEntityID, companyCode, locationID,
			fmt.Sprintf("%d", displayOrder),
			parentCode, managerUserID, managerEmail,
			effectiveDate.UTC().Format(time.RFC3339),
			endDate.UTC().Format(time.RFC3339),
		}
		if err := w.Write(rec); err != nil {
			return withCode(exitDB, err)
		}
	}
	if err := rows.Err(); err != nil {
		return withCode(exitDB, err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return withCode(exitDB, err)
	}
	return nil
}

func exportPositions(ctx context.Context, pool *pgxpool.Pool, opts exportOptions) error {
	path := filepath.Join(opts.outputDir, "positions.csv")
	f, err := os.Create(path)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	header := []string{"code", "org_node_code", "title", "status", "is_auto_created", "effective_date", "end_date"}
	if err := w.Write(header); err != nil {
		return withCode(exitDB, err)
	}

	var rows pgx.Rows
	if opts.asOf != nil {
		rows, err = pool.Query(ctx, `
			SELECT
				p.code,
				n.code AS org_node_code,
				COALESCE(p.title, ''),
				p.status,
				p.is_auto_created,
				p.effective_date,
				p.end_date
			FROM org_positions p
			JOIN org_nodes n
			  ON n.tenant_id = p.tenant_id
			 AND n.id = p.org_node_id
			WHERE p.tenant_id = $1
			  AND p.effective_date <= $2 AND p.end_date > $2
			ORDER BY p.code ASC
		`, opts.tenantID, *opts.asOf)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT
				p.code,
				n.code AS org_node_code,
				COALESCE(p.title, ''),
				p.status,
				p.is_auto_created,
				p.effective_date,
				p.end_date
			FROM org_positions p
			JOIN org_nodes n
			  ON n.tenant_id = p.tenant_id
			 AND n.id = p.org_node_id
			WHERE p.tenant_id = $1
			ORDER BY p.code ASC, p.effective_date ASC
		`, opts.tenantID)
	}
	if err != nil {
		return withCode(exitDB, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			code, orgNodeCode, title, status string
			isAutoCreated                    bool
			effectiveDate, endDate           time.Time
		)
		if err := rows.Scan(&code, &orgNodeCode, &title, &status, &isAutoCreated, &effectiveDate, &endDate); err != nil {
			return withCode(exitDB, err)
		}
		rec := []string{
			code, orgNodeCode, title, status,
			fmt.Sprintf("%t", isAutoCreated),
			effectiveDate.UTC().Format(time.RFC3339),
			endDate.UTC().Format(time.RFC3339),
		}
		if err := w.Write(rec); err != nil {
			return withCode(exitDB, err)
		}
	}
	if err := rows.Err(); err != nil {
		return withCode(exitDB, err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return withCode(exitDB, err)
	}
	return nil
}

func exportAssignments(ctx context.Context, pool *pgxpool.Pool, opts exportOptions) error {
	path := filepath.Join(opts.outputDir, "assignments.csv")
	f, err := os.Create(path)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	header := []string{"position_code", "assignment_type", "pernr", "subject_id", "effective_date", "end_date"}
	if err := w.Write(header); err != nil {
		return withCode(exitDB, err)
	}

	var rows pgx.Rows
	if opts.asOf != nil {
		rows, err = pool.Query(ctx, `
			SELECT
				p.code AS position_code,
				a.assignment_type,
				a.pernr,
				a.subject_id::text,
				a.effective_date,
				a.end_date
			FROM org_assignments a
			JOIN org_positions p
			  ON p.tenant_id = a.tenant_id
			 AND p.id = a.position_id
			WHERE a.tenant_id = $1
			  AND a.effective_date <= $2 AND a.end_date > $2
			ORDER BY a.pernr ASC, a.effective_date ASC
		`, opts.tenantID, *opts.asOf)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT
				p.code AS position_code,
				a.assignment_type,
				a.pernr,
				a.subject_id::text,
				a.effective_date,
				a.end_date
			FROM org_assignments a
			JOIN org_positions p
			  ON p.tenant_id = a.tenant_id
			 AND p.id = a.position_id
			WHERE a.tenant_id = $1
			ORDER BY a.pernr ASC, a.effective_date ASC
		`, opts.tenantID)
	}
	if err != nil {
		return withCode(exitDB, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			positionCode, assignmentType, pernr, subjectID string
			effectiveDate, endDate                         time.Time
		)
		if err := rows.Scan(&positionCode, &assignmentType, &pernr, &subjectID, &effectiveDate, &endDate); err != nil {
			return withCode(exitDB, err)
		}
		rec := []string{
			positionCode, assignmentType, pernr, subjectID,
			effectiveDate.UTC().Format(time.RFC3339),
			endDate.UTC().Format(time.RFC3339),
		}
		if err := w.Write(rec); err != nil {
			return withCode(exitDB, err)
		}
	}
	if err := rows.Err(); err != nil {
		return withCode(exitDB, err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return withCode(exitDB, err)
	}
	return nil
}
