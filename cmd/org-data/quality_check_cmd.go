package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/org/domain/subjectid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type qualityCheckOptions struct {
	tenantID  uuid.UUID
	asOf      time.Time
	backend   string
	outputDir string
	format    string
	baseURL   string
	authToken string
	maxIssues int
}

func newQualityCheckCmd() *cobra.Command {
	var (
		tenant string
		asOf   string
		opts   qualityCheckOptions
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Generate org_quality_report.v1 (DEV-PLAN-031)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			opts.backend = strings.ToLower(strings.TrimSpace(opts.backend))
			if opts.backend == "" {
				opts.backend = "db"
			}
			opts.format = strings.ToLower(strings.TrimSpace(opts.format))
			if opts.format == "" {
				opts.format = "json"
			}
			if opts.format != "json" {
				return withCode(exitUsage, fmt.Errorf("unsupported --format: %s", opts.format))
			}

			id, err := uuid.Parse(strings.TrimSpace(tenant))
			if err != nil {
				return withCode(exitUsage, fmt.Errorf("invalid --tenant: %w", err))
			}
			opts.tenantID = id

			t, err := parseTimeField(asOf)
			if err != nil && strings.TrimSpace(asOf) != "" {
				return withCode(exitUsage, fmt.Errorf("invalid --as-of: %w", err))
			}
			if t.IsZero() {
				t = time.Now().UTC()
			}
			opts.asOf = t.UTC()

			if opts.maxIssues <= 0 {
				opts.maxIssues = qualityMaxIssuesDefaultLimit
			}

			if strings.TrimSpace(opts.outputDir) == "" {
				opts.outputDir = "."
			}
			if err := ensureDir(opts.outputDir); err != nil {
				return err
			}

			report, outPath, err := runQualityCheck(ctx, opts)
			if err != nil {
				return err
			}
			if err := writeJSONFile(outPath, report); err != nil {
				return err
			}

			type summary struct {
				Status    string `json:"status"`
				RunID     string `json:"run_id"`
				TenantID  string `json:"tenant_id"`
				Backend   string `json:"backend"`
				AsOf      string `json:"as_of"`
				Output    string `json:"output"`
				Errors    int    `json:"errors"`
				Warnings  int    `json:"warnings"`
				Total     int    `json:"issues_total"`
				Truncated bool   `json:"truncated"`
			}
			return writeJSONLine(summary{
				Status:    "ok",
				RunID:     report.RunID.String(),
				TenantID:  report.TenantID.String(),
				Backend:   opts.backend,
				AsOf:      report.AsOf.UTC().Format(time.RFC3339),
				Output:    outPath,
				Errors:    report.Summary.Errors,
				Warnings:  report.Summary.Warnings,
				Total:     report.Summary.IssuesTotal,
				Truncated: report.Summary.Truncated,
			})
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant UUID (required)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "As-of time (YYYY-MM-DD or RFC3339; default now UTC)")
	cmd.Flags().StringVar(&opts.backend, "backend", "db", "Backend: db|api")
	cmd.Flags().StringVar(&opts.outputDir, "output", ".", "Output directory")
	cmd.Flags().StringVar(&opts.format, "format", "json", "Output format: json")
	cmd.Flags().IntVar(&opts.maxIssues, "max-issues", qualityMaxIssuesDefaultLimit, "Max issues to output (truncate beyond this)")

	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "Base URL for api backend (default: ORIGIN)")
	cmd.Flags().StringVar(&opts.authToken, "auth-token", "", "Authorization token for api backend (sent as Authorization header)")

	_ = cmd.MarkFlagRequired("tenant")
	return cmd
}

func runQualityCheck(ctx context.Context, opts qualityCheckOptions) (*qualityReportV1, string, error) {
	report := &qualityReportV1{
		SchemaVersion: qualityReportSchemaVersion,
		RunID:         uuid.New(),
		TenantID:      opts.tenantID,
		AsOf:          opts.asOf,
		GeneratedAt:   time.Now().UTC(),
		Ruleset: qualityRuleset{
			Name:    qualityRulesetName,
			Version: qualityRulesetVersion,
		},
		Issues: []qualityIssue{},
	}

	switch opts.backend {
	case "db":
		pool, err := connectDB(ctx)
		if err != nil {
			return nil, "", withCode(exitDB, err)
		}
		defer pool.Close()
		if err := runQualityCheckDB(ctx, pool, opts, report); err != nil {
			return nil, "", err
		}
	case "api":
		client, err := newOrgAPIClient(opts.baseURL, opts.authToken)
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(client.authorization) == "" {
			return nil, "", withCode(exitUsage, fmt.Errorf("--auth-token is required when --backend=api"))
		}
		if err := runQualityCheckAPI(ctx, client, opts, report); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", withCode(exitUsage, fmt.Errorf("unsupported --backend %q (expected db|api)", opts.backend))
	}

	sortQualityIssues(report.Issues)
	if len(report.Issues) > opts.maxIssues {
		report.Issues = report.Issues[:opts.maxIssues]
		report.Summary.Truncated = true
	}
	for _, iss := range report.Issues {
		switch iss.Severity {
		case severityError:
			report.Summary.Errors++
		case severityWarning:
			report.Summary.Warnings++
		}
	}
	report.Summary.IssuesTotal = len(report.Issues)

	outPath := qualityReportFilePath(opts.outputDir, report.TenantID, report.AsOf, report.RunID)
	return report, outPath, report.validate()
}

func runQualityCheckDB(ctx context.Context, pool *pgxpool.Pool, opts qualityCheckOptions, report *qualityReportV1) error {
	return withTenantTx(ctx, pool, opts.tenantID, func(txCtx context.Context, tx pgx.Tx) error {
		nodesAll, err := listOrgNodesAll(txCtx, tx, opts.tenantID)
		if err != nil {
			return err
		}
		positionsAll, err := listOrgPositionsAll(txCtx, tx, opts.tenantID)
		if err != nil {
			return err
		}

		nodeSlices, err := listNodeSlicesAsOf(txCtx, tx, opts.tenantID, opts.asOf)
		if err != nil {
			return err
		}
		edges, err := listEdgesAsOf(txCtx, tx, opts.tenantID, opts.asOf)
		if err != nil {
			return err
		}
		positionsAsOf, err := listPositionsAsOf(txCtx, tx, opts.tenantID, opts.asOf)
		if err != nil {
			return err
		}
		assignmentsAsOf, err := listAssignmentsAsOf(txCtx, tx, opts.tenantID, opts.asOf)
		if err != nil {
			return err
		}

		rootIDs := []uuid.UUID{}
		for _, n := range nodesAll {
			if n.IsRoot {
				rootIDs = append(rootIDs, n.ID)
			}
		}
		rootSet := map[uuid.UUID]bool{}
		for _, id := range rootIDs {
			rootSet[id] = true
		}

		// ORG_Q_001
		for _, n := range nodesAll {
			if nodeCodeRegex.MatchString(n.Code) {
				continue
			}
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleNodeCodeFormat,
				Severity: severityWarning,
				Entity:   qualityEntityRef{Type: "org_node", ID: n.ID},
				Message:  "node code does not match required format",
				Details: map[string]any{
					"code":  n.Code,
					"regex": nodeCodeRegex.String(),
				},
			})
		}

		// ORG_Q_002
		for _, p := range positionsAll {
			ok := positionCodeRegex.MatchString(p.Code)
			if p.IsAutoCreated {
				ok = autoPositionRegex.MatchString(p.Code)
			}
			if ok {
				continue
			}
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   rulePositionCodeFormat,
				Severity: severityWarning,
				Entity:   qualityEntityRef{Type: "org_position", ID: p.ID},
				Message:  "position code does not match required format",
				Details: map[string]any{
					"code":            p.Code,
					"is_auto_created": p.IsAutoCreated,
					"regex_auto":      autoPositionRegex.String(),
					"regex_general":   positionCodeRegex.String(),
				},
			})
		}

		// ORG_Q_003
		if len(rootIDs) != 1 {
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleRootInvariants,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "tenant", ID: opts.tenantID},
				Message:  "root node count must be exactly 1",
				Details: map[string]any{
					"root_count": len(rootIDs),
				},
			})
		} else {
			rootID := rootIDs[0]
			if _, ok := nodeSlices[rootID]; !ok {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleRootInvariants,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_node", ID: rootID},
					Message:  "root node is missing node slice at as-of",
				})
			}
			edgeByChild := map[uuid.UUID]edgeAsOfRow{}
			for _, e := range edges {
				edgeByChild[e.ChildNodeID] = e
			}
			rootEdge, ok := edgeByChild[rootID]
			if !ok {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleRootInvariants,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_node", ID: rootID},
					Message:  "root node is missing edge slice at as-of",
				})
			} else if rootEdge.ParentNodeID != nil {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleRootInvariants,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_edge", ID: rootEdge.EdgeID},
					Message:  "root edge must have parent_node_id=null",
					Details: map[string]any{
						"child_node_id":  rootEdge.ChildNodeID.String(),
						"parent_node_id": rootEdge.ParentNodeID.String(),
					},
				})
			}
		}

		// ORG_Q_004 / ORG_Q_005 using full node list
		edgeByChild := map[uuid.UUID]edgeAsOfRow{}
		for _, e := range edges {
			edgeByChild[e.ChildNodeID] = e
		}
		for _, n := range nodesAll {
			if _, ok := nodeSlices[n.ID]; !ok {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleNodeMissingSliceAsOf,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_node", ID: n.ID},
					Message:  "node is missing node slice at as-of",
				})
			}
			if n.IsRoot {
				continue
			}
			if _, ok := edgeByChild[n.ID]; !ok {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleNodeMissingEdgeAsOf,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_node", ID: n.ID},
					Message:  "non-root node is missing edge slice at as-of",
				})
			}
		}

		// ORG_Q_006
		for _, e := range edges {
			if e.ParentNodeID != nil {
				continue
			}
			if rootSet[e.ChildNodeID] {
				continue
			}
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleEdgeParentNullNonRoot,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_edge", ID: e.EdgeID},
				Message:  "edge has parent_node_id=null but child is not root",
				Details: map[string]any{
					"child_node_id": e.ChildNodeID.String(),
				},
			})
		}

		// ORG_Q_007
		childrenByParent := map[uuid.UUID]int{}
		for _, e := range edges {
			if e.ParentNodeID == nil {
				continue
			}
			childrenByParent[*e.ParentNodeID]++
		}
		activePositionsByNode := map[uuid.UUID]int{}
		for _, p := range positionsAsOf {
			if p.Status != "active" {
				continue
			}
			activePositionsByNode[p.OrgNodeID]++
		}
		for nodeID, slice := range nodeSlices {
			if strings.TrimSpace(slice.Status) != "active" {
				continue
			}
			if childrenByParent[nodeID] > 0 {
				continue
			}
			if activePositionsByNode[nodeID] > 0 {
				continue
			}
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleLeafRequiresPositionAsOf,
				Severity: severityWarning,
				Entity:   qualityEntityRef{Type: "org_node", ID: nodeID},
				EffectiveWindow: &qualityEffectiveWindow{
					EffectiveDate: slice.EffectiveDate,
					EndDate:       slice.EndDate,
				},
				Message: "leaf active node requires at least one active position at as-of",
			})
		}

		// ORG_Q_008
		for _, a := range assignmentsAsOf {
			pernrTrim := strings.TrimSpace(a.Pernr)
			expected, err := subjectid.NormalizedSubjectID(opts.tenantID, a.SubjectType, pernrTrim)
			if err != nil {
				report.Issues = append(report.Issues, qualityIssue{
					IssueID:  uuid.New(),
					RuleID:   ruleAssignmentSubjectMapping,
					Severity: severityError,
					Entity:   qualityEntityRef{Type: "org_assignment", ID: a.ID},
					EffectiveWindow: &qualityEffectiveWindow{
						EffectiveDate: a.EffectiveDate,
						EndDate:       a.EndDate,
					},
					Message: "subject_id mapping failed",
					Details: map[string]any{
						"pernr": a.Pernr,
						"err":   err.Error(),
					},
				})
				continue
			}
			if a.SubjectID == expected && a.Pernr == pernrTrim {
				continue
			}

			autofix := (*qualityAutofix)(nil)
			if a.AssignmentType == "primary" && a.PositionID != uuid.Nil {
				autofix = &qualityAutofix{Supported: true, FixKind: fixKindAssignmentCorrect, Risk: "low"}
			}
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleAssignmentSubjectMapping,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_assignment", ID: a.ID},
				EffectiveWindow: &qualityEffectiveWindow{
					EffectiveDate: a.EffectiveDate,
					EndDate:       a.EndDate,
				},
				Message: "subject_id mismatch with SSOT mapping",
				Details: map[string]any{
					"pernr":               a.Pernr,
					"pernr_trim":          pernrTrim,
					"expected_subject_id": expected.String(),
					"actual_subject_id":   a.SubjectID.String(),
					"position_id":         a.PositionID.String(),
					"assignment_type":     a.AssignmentType,
				},
				Autofix: autofix,
			})
		}

		return nil
	})
}

type orgNodeRow struct {
	ID     uuid.UUID
	Code   string
	IsRoot bool
}

type orgPositionRow struct {
	ID            uuid.UUID
	Code          string
	IsAutoCreated bool
}

type nodeSliceAsOfRow struct {
	OrgNodeID     uuid.UUID
	Status        string
	EffectiveDate time.Time
	EndDate       time.Time
}

type edgeAsOfRow struct {
	EdgeID        uuid.UUID
	ParentNodeID  *uuid.UUID
	ChildNodeID   uuid.UUID
	EffectiveDate time.Time
	EndDate       time.Time
}

type positionAsOfRow struct {
	ID            uuid.UUID
	OrgNodeID     uuid.UUID
	Status        string
	EffectiveDate time.Time
	EndDate       time.Time
}

type assignmentAsOfRow struct {
	ID             uuid.UUID
	PositionID     uuid.UUID
	SubjectType    string
	SubjectID      uuid.UUID
	Pernr          string
	AssignmentType string
	EffectiveDate  time.Time
	EndDate        time.Time
}

func withTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(txCtx context.Context, tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return withCode(exitDB, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTx(ctx, tx)
	txCtx = composables.WithTenantID(txCtx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return withCode(exitDB, fmt.Errorf("apply tenant rls: %w", err))
	}
	if err := fn(txCtx, tx); err != nil {
		return err
	}
	return nil
}

func listOrgNodesAll(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) ([]orgNodeRow, error) {
	rows, err := tx.Query(ctx, `SELECT id, code, is_root FROM org_nodes WHERE tenant_id=$1 ORDER BY id ASC`, tenantID)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_nodes: %w", err))
	}
	defer rows.Close()
	out := []orgNodeRow{}
	for rows.Next() {
		var r orgNodeRow
		if err := rows.Scan(&r.ID, &r.Code, &r.IsRoot); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_nodes: %w", err))
		}
		out = append(out, r)
	}
	return out, withCode(exitDB, rows.Err())
}

func listOrgPositionsAll(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) ([]orgPositionRow, error) {
	rows, err := tx.Query(ctx, `SELECT id, code, is_auto_created FROM org_positions WHERE tenant_id=$1 ORDER BY id ASC`, tenantID)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_positions: %w", err))
	}
	defer rows.Close()
	out := []orgPositionRow{}
	for rows.Next() {
		var r orgPositionRow
		if err := rows.Scan(&r.ID, &r.Code, &r.IsAutoCreated); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_positions: %w", err))
		}
		out = append(out, r)
	}
	return out, withCode(exitDB, rows.Err())
}

func listNodeSlicesAsOf(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, asOf time.Time) (map[uuid.UUID]nodeSliceAsOfRow, error) {
	rows, err := tx.Query(ctx, `
SELECT org_node_id, status, effective_date, end_date
FROM org_node_slices
WHERE tenant_id=$1 AND effective_date <= $2 AND end_date > $2
`, tenantID, asOf)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_node_slices as-of: %w", err))
	}
	defer rows.Close()
	out := map[uuid.UUID]nodeSliceAsOfRow{}
	for rows.Next() {
		var r nodeSliceAsOfRow
		if err := rows.Scan(&r.OrgNodeID, &r.Status, &r.EffectiveDate, &r.EndDate); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_node_slices: %w", err))
		}
		out[r.OrgNodeID] = r
	}
	return out, withCode(exitDB, rows.Err())
}

func listEdgesAsOf(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, asOf time.Time) ([]edgeAsOfRow, error) {
	rows, err := tx.Query(ctx, `
SELECT id, parent_node_id, child_node_id, effective_date, end_date
FROM org_edges
WHERE tenant_id=$1
  AND hierarchy_type='OrgUnit'
  AND effective_date <= $2 AND end_date > $2
ORDER BY id ASC
`, tenantID, asOf)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_edges as-of: %w", err))
	}
	defer rows.Close()
	out := []edgeAsOfRow{}
	for rows.Next() {
		var r edgeAsOfRow
		if err := rows.Scan(&r.EdgeID, &r.ParentNodeID, &r.ChildNodeID, &r.EffectiveDate, &r.EndDate); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_edges: %w", err))
		}
		out = append(out, r)
	}
	return out, withCode(exitDB, rows.Err())
}

func listPositionsAsOf(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, asOf time.Time) ([]positionAsOfRow, error) {
	rows, err := tx.Query(ctx, `
SELECT id, org_node_id, status, effective_date, end_date
FROM org_positions
WHERE tenant_id=$1 AND effective_date <= $2 AND end_date > $2
ORDER BY id ASC
`, tenantID, asOf)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_positions as-of: %w", err))
	}
	defer rows.Close()
	out := []positionAsOfRow{}
	for rows.Next() {
		var r positionAsOfRow
		if err := rows.Scan(&r.ID, &r.OrgNodeID, &r.Status, &r.EffectiveDate, &r.EndDate); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_positions as-of: %w", err))
		}
		out = append(out, r)
	}
	return out, withCode(exitDB, rows.Err())
}

func listAssignmentsAsOf(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, asOf time.Time) ([]assignmentAsOfRow, error) {
	rows, err := tx.Query(ctx, `
SELECT id, position_id, subject_type, subject_id, pernr, assignment_type, effective_date, end_date
FROM org_assignments
WHERE tenant_id=$1 AND effective_date <= $2 AND end_date > $2
ORDER BY id ASC
`, tenantID, asOf)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("list org_assignments as-of: %w", err))
	}
	defer rows.Close()
	out := []assignmentAsOfRow{}
	for rows.Next() {
		var r assignmentAsOfRow
		if err := rows.Scan(&r.ID, &r.PositionID, &r.SubjectType, &r.SubjectID, &r.Pernr, &r.AssignmentType, &r.EffectiveDate, &r.EndDate); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("scan org_assignments as-of: %w", err))
		}
		out = append(out, r)
	}
	return out, withCode(exitDB, rows.Err())
}

type snapshotNodeValues struct {
	OrgNodeID     uuid.UUID  `json:"org_node_id"`
	IsRoot        bool       `json:"is_root"`
	Code          string     `json:"code"`
	Status        string     `json:"status"`
	ParentNodeID  *uuid.UUID `json:"parent_node_id"`
	EffectiveDate time.Time  `json:"effective_date"`
	EndDate       time.Time  `json:"end_date"`
}

type snapshotEdgeValues struct {
	EdgeID        uuid.UUID  `json:"edge_id"`
	ParentNodeID  *uuid.UUID `json:"parent_node_id"`
	ChildNodeID   uuid.UUID  `json:"child_node_id"`
	EffectiveDate time.Time  `json:"effective_date"`
	EndDate       time.Time  `json:"end_date"`
}

type snapshotPositionValues struct {
	OrgPositionID uuid.UUID `json:"org_position_id"`
	OrgNodeID     uuid.UUID `json:"org_node_id"`
	Code          string    `json:"code"`
	Status        string    `json:"status"`
	IsAutoCreated bool      `json:"is_auto_created"`
	EffectiveDate time.Time `json:"effective_date"`
	EndDate       time.Time `json:"end_date"`
}

type snapshotAssignmentValues struct {
	OrgAssignmentID uuid.UUID `json:"org_assignment_id"`
	PositionID      uuid.UUID `json:"position_id"`
	SubjectType     string    `json:"subject_type"`
	SubjectID       uuid.UUID `json:"subject_id"`
	Pernr           string    `json:"pernr"`
	AssignmentType  string    `json:"assignment_type"`
	EffectiveDate   time.Time `json:"effective_date"`
	EndDate         time.Time `json:"end_date"`
}

func runQualityCheckAPI(ctx context.Context, client *orgAPIClient, opts qualityCheckOptions, report *qualityReportV1) error {
	res, err := client.getSnapshotAll(ctx, opts.asOf.Format(time.RFC3339), []string{"nodes", "edges", "positions", "assignments"})
	if err != nil {
		return err
	}
	if res.TenantID != opts.tenantID {
		return withCode(exitValidation, fmt.Errorf("snapshot tenant_id=%s does not match --tenant=%s", res.TenantID, opts.tenantID))
	}

	nodes := map[uuid.UUID]snapshotNodeValues{}
	edges := []snapshotEdgeValues{}
	positions := []snapshotPositionValues{}
	assignments := []snapshotAssignmentValues{}

	for _, it := range res.Items {
		switch it.EntityType {
		case "org_node":
			var v snapshotNodeValues
			if err := json.Unmarshal(it.NewValues, &v); err != nil {
				return withCode(exitDB, fmt.Errorf("snapshot decode org_node: %w", err))
			}
			nodes[v.OrgNodeID] = v
		case "org_edge":
			var v snapshotEdgeValues
			if err := json.Unmarshal(it.NewValues, &v); err != nil {
				return withCode(exitDB, fmt.Errorf("snapshot decode org_edge: %w", err))
			}
			edges = append(edges, v)
		case "org_position":
			var v snapshotPositionValues
			if err := json.Unmarshal(it.NewValues, &v); err != nil {
				return withCode(exitDB, fmt.Errorf("snapshot decode org_position: %w", err))
			}
			positions = append(positions, v)
		case "org_assignment":
			var v snapshotAssignmentValues
			if err := json.Unmarshal(it.NewValues, &v); err != nil {
				return withCode(exitDB, fmt.Errorf("snapshot decode org_assignment: %w", err))
			}
			assignments = append(assignments, v)
		}
	}

	rootIDs := []uuid.UUID{}
	for id, n := range nodes {
		if n.IsRoot {
			rootIDs = append(rootIDs, id)
		}
	}
	rootSet := map[uuid.UUID]bool{}
	for _, id := range rootIDs {
		rootSet[id] = true
	}

	// ORG_Q_001 (as-of nodes only)
	for _, n := range nodes {
		if nodeCodeRegex.MatchString(n.Code) {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleNodeCodeFormat,
			Severity: severityWarning,
			Entity:   qualityEntityRef{Type: "org_node", ID: n.OrgNodeID},
			Message:  "node code does not match required format",
			Details: map[string]any{
				"code":  n.Code,
				"regex": nodeCodeRegex.String(),
			},
		})
	}

	// ORG_Q_002 (as-of positions only)
	for _, p := range positions {
		ok := positionCodeRegex.MatchString(p.Code)
		if p.IsAutoCreated {
			ok = autoPositionRegex.MatchString(p.Code)
		}
		if ok {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   rulePositionCodeFormat,
			Severity: severityWarning,
			Entity:   qualityEntityRef{Type: "org_position", ID: p.OrgPositionID},
			Message:  "position code does not match required format",
			Details: map[string]any{
				"code":            p.Code,
				"is_auto_created": p.IsAutoCreated,
				"regex_auto":      autoPositionRegex.String(),
				"regex_general":   positionCodeRegex.String(),
			},
		})
	}

	edgeByChild := map[uuid.UUID]snapshotEdgeValues{}
	childrenByParent := map[uuid.UUID]int{}
	nodesReferenced := map[uuid.UUID]bool{}
	for _, e := range edges {
		edgeByChild[e.ChildNodeID] = e
		nodesReferenced[e.ChildNodeID] = true
		if e.ParentNodeID != nil {
			childrenByParent[*e.ParentNodeID]++
			nodesReferenced[*e.ParentNodeID] = true
		}
	}
	for _, p := range positions {
		nodesReferenced[p.OrgNodeID] = true
	}

	// ORG_Q_003
	if len(rootIDs) != 1 {
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleRootInvariants,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "tenant", ID: opts.tenantID},
			Message:  "root node count must be exactly 1 (as-of snapshot)",
			Details: map[string]any{
				"root_count": len(rootIDs),
			},
		})
	} else {
		rootID := rootIDs[0]
		rootEdge, ok := edgeByChild[rootID]
		if !ok {
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleRootInvariants,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_node", ID: rootID},
				Message:  "root node is missing edge slice at as-of",
			})
		} else if rootEdge.ParentNodeID != nil {
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleRootInvariants,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_edge", ID: rootEdge.EdgeID},
				Message:  "root edge must have parent_node_id=null",
			})
		}
	}

	// ORG_Q_004 best-effort: referenced nodes not present in snapshot nodes.
	for nodeID := range nodesReferenced {
		if _, ok := nodes[nodeID]; ok {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleNodeMissingSliceAsOf,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "org_node", ID: nodeID},
			Message:  "node is missing node slice at as-of (inferred from snapshot references)",
		})
	}

	// ORG_Q_005
	for _, n := range nodes {
		if n.IsRoot {
			continue
		}
		if _, ok := edgeByChild[n.OrgNodeID]; ok {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleNodeMissingEdgeAsOf,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "org_node", ID: n.OrgNodeID},
			Message:  "non-root node is missing edge slice at as-of",
		})
	}

	// ORG_Q_006
	for _, e := range edges {
		if e.ParentNodeID != nil {
			continue
		}
		if rootSet[e.ChildNodeID] {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleEdgeParentNullNonRoot,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "org_edge", ID: e.EdgeID},
			Message:  "edge has parent_node_id=null but child is not root",
			Details: map[string]any{
				"child_node_id": e.ChildNodeID.String(),
			},
		})
	}

	// ORG_Q_007
	activePositionsByNode := map[uuid.UUID]int{}
	for _, p := range positions {
		if strings.TrimSpace(p.Status) != "active" {
			continue
		}
		activePositionsByNode[p.OrgNodeID]++
	}
	for _, n := range nodes {
		if strings.TrimSpace(n.Status) != "active" {
			continue
		}
		if childrenByParent[n.OrgNodeID] > 0 {
			continue
		}
		if activePositionsByNode[n.OrgNodeID] > 0 {
			continue
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleLeafRequiresPositionAsOf,
			Severity: severityWarning,
			Entity:   qualityEntityRef{Type: "org_node", ID: n.OrgNodeID},
			EffectiveWindow: &qualityEffectiveWindow{
				EffectiveDate: n.EffectiveDate,
				EndDate:       n.EndDate,
			},
			Message: "leaf active node requires at least one active position at as-of",
		})
	}

	// ORG_Q_008
	for _, a := range assignments {
		pernrTrim := strings.TrimSpace(a.Pernr)
		expected, err := subjectid.NormalizedSubjectID(opts.tenantID, a.SubjectType, pernrTrim)
		if err != nil {
			report.Issues = append(report.Issues, qualityIssue{
				IssueID:  uuid.New(),
				RuleID:   ruleAssignmentSubjectMapping,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_assignment", ID: a.OrgAssignmentID},
				EffectiveWindow: &qualityEffectiveWindow{
					EffectiveDate: a.EffectiveDate,
					EndDate:       a.EndDate,
				},
				Message: "subject_id mapping failed",
				Details: map[string]any{
					"pernr": a.Pernr,
					"err":   err.Error(),
				},
			})
			continue
		}
		if a.SubjectID == expected && a.Pernr == pernrTrim {
			continue
		}
		autofix := (*qualityAutofix)(nil)
		if a.AssignmentType == "primary" && a.PositionID != uuid.Nil {
			autofix = &qualityAutofix{Supported: true, FixKind: fixKindAssignmentCorrect, Risk: "low"}
		}
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleAssignmentSubjectMapping,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "org_assignment", ID: a.OrgAssignmentID},
			EffectiveWindow: &qualityEffectiveWindow{
				EffectiveDate: a.EffectiveDate,
				EndDate:       a.EndDate,
			},
			Message: "subject_id mismatch with SSOT mapping",
			Details: map[string]any{
				"pernr":               a.Pernr,
				"pernr_trim":          pernrTrim,
				"expected_subject_id": expected.String(),
				"actual_subject_id":   a.SubjectID.String(),
				"position_id":         a.PositionID.String(),
				"assignment_type":     a.AssignmentType,
			},
			Autofix: autofix,
		})
	}

	return nil
}
