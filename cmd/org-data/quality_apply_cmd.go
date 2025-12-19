package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type qualityApplyOptions struct {
	fixPlanPath     string
	outputDir       string
	dryRun          bool
	apply           bool
	yes             bool
	changeRequestID string
	beforeBackend   string
	baseURL         string
	authToken       string
}

func newQualityApplyCmd() *cobra.Command {
	var opts qualityApplyOptions

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply org_quality_fix_plan.v1 via /org/api/batch (DEV-PLAN-031)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stringsTrim(opts.fixPlanPath) == "" {
				return withCode(exitUsage, fmt.Errorf("--fix-plan is required"))
			}
			if stringsTrim(opts.outputDir) == "" {
				opts.outputDir = "."
			}
			if err := ensureDir(opts.outputDir); err != nil {
				return err
			}

			opts.beforeBackend = strings.ToLower(strings.TrimSpace(opts.beforeBackend))
			if opts.beforeBackend == "" {
				opts.beforeBackend = "api"
			}
			switch opts.beforeBackend {
			case "api", "db":
			default:
				return withCode(exitUsage, fmt.Errorf("unsupported --before-backend %q (expected api|db)", opts.beforeBackend))
			}

			conf := configuration.Use()
			if !conf.OrgDataQualityEnabled {
				return withCode(exitUsage, fmt.Errorf("ORG_DATA_QUALITY_ENABLED=false: quality apply is disabled"))
			}

			if opts.dryRun && opts.apply {
				return withCode(exitUsage, fmt.Errorf("--dry-run and --apply are mutually exclusive"))
			}
			if opts.dryRun {
				opts.apply = false
			}

			if opts.apply && !opts.yes {
				return withCode(exitUsage, fmt.Errorf("--apply requires --yes"))
			}

			var fixPlan qualityFixPlanV1
			if err := readJSONFile(opts.fixPlanPath, &fixPlan); err != nil {
				return err
			}
			if err := fixPlan.validate(); err != nil {
				return err
			}
			if conf.OrgDataFixesMaxCommands > 0 && len(fixPlan.BatchRequest.Commands) > conf.OrgDataFixesMaxCommands {
				return withCode(exitValidation, fmt.Errorf("fix plan too large: %d commands > ORG_DATA_FIXES_MAX_COMMANDS=%d", len(fixPlan.BatchRequest.Commands), conf.OrgDataFixesMaxCommands))
			}

			client, err := newOrgAPIClient(opts.baseURL, opts.authToken)
			if err != nil {
				return err
			}
			if strings.TrimSpace(client.authorization) == "" {
				return withCode(exitUsage, fmt.Errorf("--auth-token is required"))
			}

			manifest := &qualityFixManifestV1{
				SchemaVersion:      qualityFixManSchemaVersion,
				RunID:              uuid.New(),
				TenantID:           fixPlan.TenantID,
				AsOf:               fixPlan.AsOf,
				AppliedAt:          time.Now().UTC(),
				SourceFixPlanRunID: fixPlan.RunID,
				ChangeRequestID:    nil,
				BatchRequest: qualityBatchRequest{
					DryRun:        !opts.apply,
					EffectiveDate: fixPlan.BatchRequest.EffectiveDate,
					Commands:      fixPlan.BatchRequest.Commands,
				},
				Before:  qualityFixBefore{Assignments: []qualityBeforeAssignment{}},
				Results: qualityFixResults{Ok: false, EventsEnqueued: 0, BatchResults: nil, Error: nil},
			}

			if stringsTrim(opts.changeRequestID) != "" {
				id, err := uuid.Parse(stringsTrim(opts.changeRequestID))
				if err != nil {
					return withCode(exitUsage, fmt.Errorf("invalid --change-request-id: %w", err))
				}
				manifest.ChangeRequestID = &id

				crPayload, err := client.getChangeRequest(cmd.Context(), id)
				if err != nil {
					return err
				}
				if err := ensureChangeRequestPayloadMatchesFixPlan(crPayload, fixPlan.BatchRequest); err != nil {
					return err
				}

				preflightRaw, err := client.postPreflight(cmd.Context(), fixPlan.BatchRequest.EffectiveDate, fixPlan.BatchRequest.Commands)
				if err != nil {
					return err
				}
				manifest.PreflightResponseRaw = preflightRaw
			}

			before, err := readBeforeAssignments(cmd.Context(), opts.beforeBackend, fixPlan, client)
			if err != nil {
				return err
			}
			manifest.Before.Assignments = before

			results, err := client.postBatch(cmd.Context(), manifest.BatchRequest)
			if err != nil {
				return err
			}
			manifest.Results = results

			outPath := qualityFixManifestFilePath(opts.outputDir, manifest.TenantID, manifest.AsOf, manifest.RunID)
			if err := writeJSONFile(outPath, manifest); err != nil {
				return err
			}

			if !manifest.Results.Ok {
				if manifest.Results.Error == nil {
					return withCode(exitDBWrite, fmt.Errorf("batch failed"))
				}
				return withCode(exitDBWrite, fmt.Errorf("batch failed: %s (%s)", manifest.Results.Error.Message, manifest.Results.Error.Code))
			}

			type summary struct {
				Status         string `json:"status"`
				RunID          string `json:"run_id"`
				TenantID       string `json:"tenant_id"`
				AsOf           string `json:"as_of"`
				DryRun         bool   `json:"dry_run"`
				Commands       int    `json:"commands"`
				EventsEnqueued int    `json:"events_enqueued"`
				Manifest       string `json:"manifest"`
			}
			return writeJSONLine(summary{
				Status:         "ok",
				RunID:          manifest.RunID.String(),
				TenantID:       manifest.TenantID.String(),
				AsOf:           manifest.AsOf.UTC().Format(time.RFC3339),
				DryRun:         manifest.BatchRequest.DryRun,
				Commands:       len(manifest.BatchRequest.Commands),
				EventsEnqueued: manifest.Results.EventsEnqueued,
				Manifest:       outPath,
			})
		},
	}

	cmd.Flags().StringVar(&opts.fixPlanPath, "fix-plan", "", "Path to org_quality_fix_plan.v1.json (required)")
	cmd.Flags().StringVar(&opts.outputDir, "output", ".", "Output directory for manifest")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Force dry-run (mutually exclusive with --apply)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "Apply changes (default is dry-run)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Confirm applying changes")
	cmd.Flags().StringVar(&opts.changeRequestID, "change-request-id", "", "Optional change request id to bind & preflight")
	cmd.Flags().StringVar(&opts.beforeBackend, "before-backend", "api", "Read before-state via: api|db")

	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "Base URL for org api (default: ORIGIN)")
	cmd.Flags().StringVar(&opts.authToken, "auth-token", "", "Authorization token (sent as Authorization header)")

	_ = cmd.MarkFlagRequired("fix-plan")
	return cmd
}

func ensureChangeRequestPayloadMatchesFixPlan(changeRequestPayload json.RawMessage, planReq qualityBatchRequest) error {
	var crAny any
	if err := json.Unmarshal(changeRequestPayload, &crAny); err != nil {
		return withCode(exitValidation, fmt.Errorf("change request payload is invalid json: %w", err))
	}
	var planAny any
	b, err := json.Marshal(planReq)
	if err != nil {
		return withCode(exitValidation, fmt.Errorf("fix plan payload marshal failed: %w", err))
	}
	if err := json.Unmarshal(b, &planAny); err != nil {
		return withCode(exitValidation, fmt.Errorf("fix plan payload is invalid json: %w", err))
	}

	normalize := func(v any) any {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		delete(m, "dry_run")
		return m
	}

	if !reflect.DeepEqual(normalize(crAny), normalize(planAny)) {
		return withCode(exitValidation, fmt.Errorf("change request payload does not match fix plan batch_request (ignoring dry_run)"))
	}
	return nil
}

type assignmentCorrectPayload struct {
	ID         uuid.UUID  `json:"id"`
	Pernr      *string    `json:"pernr"`
	SubjectID  *uuid.UUID `json:"subject_id"`
	PositionID *uuid.UUID `json:"position_id"`
}

func readBeforeAssignments(ctx context.Context, beforeBackend string, fixPlan qualityFixPlanV1, client *orgAPIClient) ([]qualityBeforeAssignment, error) {
	ids := make([]uuid.UUID, 0, len(fixPlan.BatchRequest.Commands))
	for i, cmd := range fixPlan.BatchRequest.Commands {
		if stringsTrim(cmd.Type) != fixKindAssignmentCorrect {
			return nil, withCode(exitValidation, fmt.Errorf("fix_plan.commands[%d].type is unsupported", i))
		}
		var payload assignmentCorrectPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return nil, withCode(exitValidation, fmt.Errorf("fix_plan.commands[%d].payload invalid: %w", i, err))
		}
		if payload.ID == uuid.Nil {
			return nil, withCode(exitValidation, fmt.Errorf("fix_plan.commands[%d].payload.id is required", i))
		}
		ids = append(ids, payload.ID)
	}

	switch beforeBackend {
	case "api":
		return readBeforeAssignmentsAPI(ctx, client, fixPlan, ids)
	case "db":
		pool, err := connectDB(ctx)
		if err != nil {
			return nil, withCode(exitDB, err)
		}
		defer pool.Close()
		return readBeforeAssignmentsDB(ctx, pool, fixPlan.TenantID, ids)
	default:
		return nil, withCode(exitUsage, fmt.Errorf("unsupported --before-backend: %s", beforeBackend))
	}
}

func readBeforeAssignmentsAPI(ctx context.Context, client *orgAPIClient, fixPlan qualityFixPlanV1, ids []uuid.UUID) ([]qualityBeforeAssignment, error) {
	res, err := client.getSnapshotAll(ctx, fixPlan.AsOf.UTC().Format(time.RFC3339), []string{"assignments"})
	if err != nil {
		return nil, err
	}
	if res.TenantID != fixPlan.TenantID {
		return nil, withCode(exitValidation, fmt.Errorf("snapshot tenant_id=%s does not match fix plan tenant_id=%s", res.TenantID, fixPlan.TenantID))
	}

	byID := map[uuid.UUID]snapshotAssignmentValues{}
	for _, it := range res.Items {
		if it.EntityType != "org_assignment" {
			continue
		}
		var v snapshotAssignmentValues
		if err := json.Unmarshal(it.NewValues, &v); err != nil {
			return nil, withCode(exitDB, fmt.Errorf("snapshot decode org_assignment: %w", err))
		}
		byID[v.OrgAssignmentID] = v
	}

	out := make([]qualityBeforeAssignment, 0, len(ids))
	for _, id := range ids {
		v, ok := byID[id]
		if !ok {
			return nil, withCode(exitValidation, fmt.Errorf("cannot read before-state for assignment %s via api snapshot at as-of", id))
		}
		out = append(out, qualityBeforeAssignment{
			ID:         id,
			Pernr:      v.Pernr,
			SubjectID:  v.SubjectID,
			PositionID: v.PositionID,
		})
	}
	return out, nil
}

func readBeforeAssignmentsDB(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, ids []uuid.UUID) ([]qualityBeforeAssignment, error) {
	out := []qualityBeforeAssignment{}
	err := withTenantTx(ctx, pool, tenantID, func(txCtx context.Context, tx pgx.Tx) error {
		for _, id := range ids {
			var pernr string
			var subjectID uuid.UUID
			var positionID uuid.UUID
			if err := tx.QueryRow(txCtx, `SELECT pernr, subject_id, position_id FROM org_assignments WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&pernr, &subjectID, &positionID); err != nil {
				return withCode(exitDB, fmt.Errorf("read org_assignments(id=%s): %w", id, err))
			}
			out = append(out, qualityBeforeAssignment{
				ID:         id,
				Pernr:      pernr,
				SubjectID:  subjectID,
				PositionID: positionID,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
