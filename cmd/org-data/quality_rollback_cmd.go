package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

type qualityRollbackOptions struct {
	manifestPath string
	dryRun       bool
	apply        bool
	yes          bool
	baseURL      string
	authToken    string
}

func newQualityRollbackCmd() *cobra.Command {
	var opts qualityRollbackOptions

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback using org_quality_fix_manifest.v1 (DEV-PLAN-031)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stringsTrim(opts.manifestPath) == "" {
				return withCode(exitUsage, fmt.Errorf("--manifest is required"))
			}
			conf := configuration.Use()
			if !conf.OrgDataQualityEnabled {
				return withCode(exitUsage, fmt.Errorf("ORG_DATA_QUALITY_ENABLED=false: quality rollback is disabled"))
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

			var manifest qualityFixManifestV1
			if err := readJSONFile(opts.manifestPath, &manifest); err != nil {
				return err
			}
			if err := manifest.validate(); err != nil {
				return err
			}

			client, err := newOrgAPIClient(opts.baseURL, opts.authToken)
			if err != nil {
				return err
			}
			if strings.TrimSpace(client.authorization) == "" {
				return withCode(exitUsage, fmt.Errorf("--auth-token is required"))
			}

			rollbackReq, err := buildRollbackBatchRequest(manifest, !opts.apply)
			if err != nil {
				return err
			}

			results, err := client.postBatch(cmd.Context(), rollbackReq)
			if err != nil {
				return err
			}
			if !results.Ok {
				if results.Error == nil {
					return withCode(exitDBWrite, fmt.Errorf("batch failed"))
				}
				return withCode(exitDBWrite, fmt.Errorf("batch failed: %s (%s)", results.Error.Message, results.Error.Code))
			}

			type summary struct {
				Status         string `json:"status"`
				TenantID       string `json:"tenant_id"`
				AsOf           string `json:"as_of"`
				DryRun         bool   `json:"dry_run"`
				Commands       int    `json:"commands"`
				EventsEnqueued int    `json:"events_enqueued"`
				SourceManifest string `json:"source_manifest"`
			}
			return writeJSONLine(summary{
				Status:         "ok",
				TenantID:       manifest.TenantID.String(),
				AsOf:           manifest.AsOf.UTC().Format(time.RFC3339),
				DryRun:         rollbackReq.DryRun,
				Commands:       len(rollbackReq.Commands),
				EventsEnqueued: results.EventsEnqueued,
				SourceManifest: opts.manifestPath,
			})
		},
	}

	cmd.Flags().StringVar(&opts.manifestPath, "manifest", "", "Path to org_quality_fix_manifest.v1.json (required)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Force dry-run (mutually exclusive with --apply)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "Apply rollback (default is dry-run)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Confirm applying rollback")
	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "Base URL for org api (default: ORIGIN)")
	cmd.Flags().StringVar(&opts.authToken, "auth-token", "", "Authorization token (sent as Authorization header)")

	_ = cmd.MarkFlagRequired("manifest")
	return cmd
}

func buildRollbackBatchRequest(manifest qualityFixManifestV1, dryRun bool) (qualityBatchRequest, error) {
	if len(manifest.Before.Assignments) == 0 {
		return qualityBatchRequest{}, withCode(exitValidation, fmt.Errorf("manifest.before.assignments is empty"))
	}
	cmds := make([]qualityBatchCommand, 0, len(manifest.Before.Assignments))
	for _, a := range manifest.Before.Assignments {
		pernr := a.Pernr
		payload := assignmentCorrectPayload{
			ID:         a.ID,
			Pernr:      &pernr,
			SubjectID:  &a.SubjectID,
			PositionID: &a.PositionID,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return qualityBatchRequest{}, withCode(exitValidation, fmt.Errorf("marshal rollback payload: %w", err))
		}
		cmds = append(cmds, qualityBatchCommand{Type: fixKindAssignmentCorrect, Payload: raw})
	}
	return qualityBatchRequest{
		DryRun:        dryRun,
		EffectiveDate: qualityAsOfDateString(manifest.AsOf),
		Commands:      cmds,
	}, nil
}
