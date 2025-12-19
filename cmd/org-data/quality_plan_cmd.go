package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

var errNoAutofixIssues = errors.New("no autofix issues found in report")

type qualityPlanOptions struct {
	reportPath  string
	outputPath  string
	maxCommands int
}

func newQualityPlanCmd() *cobra.Command {
	var opts qualityPlanOptions

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Generate org_quality_fix_plan.v1 from a report (DEV-PLAN-031)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stringsTrim(opts.reportPath) == "" {
				return withCode(exitUsage, fmt.Errorf("--report is required"))
			}
			if stringsTrim(opts.outputPath) == "" {
				return withCode(exitUsage, fmt.Errorf("--output is required"))
			}
			if opts.maxCommands <= 0 {
				opts.maxCommands = configuration.Use().OrgDataFixesMaxCommands
			}
			if opts.maxCommands <= 0 {
				opts.maxCommands = 100
			}

			var report qualityReportV1
			if err := readJSONFile(opts.reportPath, &report); err != nil {
				return err
			}
			if err := report.validate(); err != nil {
				return err
			}

			fixPlan, err := generateFixPlanFromReport(&report, opts.maxCommands)
			if err != nil {
				if errors.Is(err, errNoAutofixIssues) && fixPlan != nil {
					type summary struct {
						Status       string `json:"status"`
						RunID        string `json:"run_id"`
						TenantID     string `json:"tenant_id"`
						AsOf         string `json:"as_of"`
						SourceReport string `json:"source_report_run_id"`
						Commands     int    `json:"commands"`
						Output       string `json:"output"`
						MaxCommands  int    `json:"max_commands"`
					}
					return writeJSONLine(summary{
						Status:       "noop",
						RunID:        fixPlan.RunID.String(),
						TenantID:     fixPlan.TenantID.String(),
						AsOf:         fixPlan.AsOf.UTC().Format(time.RFC3339),
						SourceReport: fixPlan.SourceReportRunID.String(),
						Commands:     0,
						Output:       "",
						MaxCommands:  opts.maxCommands,
					})
				}
				return err
			}

			outPath, err := resolveOutputFilePath(opts.outputPath, qualityFixPlanFilePath("", fixPlan.TenantID, fixPlan.AsOf, fixPlan.RunID))
			if err != nil {
				return err
			}
			if err := writeJSONFile(outPath, fixPlan); err != nil {
				return err
			}

			type summary struct {
				Status       string `json:"status"`
				RunID        string `json:"run_id"`
				TenantID     string `json:"tenant_id"`
				AsOf         string `json:"as_of"`
				SourceReport string `json:"source_report_run_id"`
				Commands     int    `json:"commands"`
				Output       string `json:"output"`
				MaxCommands  int    `json:"max_commands"`
			}
			return writeJSONLine(summary{
				Status:       "ok",
				RunID:        fixPlan.RunID.String(),
				TenantID:     fixPlan.TenantID.String(),
				AsOf:         fixPlan.AsOf.UTC().Format(time.RFC3339),
				SourceReport: fixPlan.SourceReportRunID.String(),
				Commands:     len(fixPlan.BatchRequest.Commands),
				Output:       outPath,
				MaxCommands:  opts.maxCommands,
			})
		},
	}

	cmd.Flags().StringVar(&opts.reportPath, "report", "", "Path to org_quality_report.v1.json (required)")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Output file or directory for fix plan (required)")
	cmd.Flags().IntVar(&opts.maxCommands, "max-commands", 0, "Max commands allowed in fix plan (default: ORG_DATA_FIXES_MAX_COMMANDS)")
	_ = cmd.MarkFlagRequired("report")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func generateFixPlanFromReport(report *qualityReportV1, maxCommands int) (*qualityFixPlanV1, error) {
	if report == nil {
		return nil, withCode(exitValidation, fmt.Errorf("report is nil"))
	}
	if err := report.validate(); err != nil {
		return nil, err
	}
	if maxCommands <= 0 {
		return nil, withCode(exitValidation, fmt.Errorf("max_commands must be positive"))
	}

	runID := uuid.New()
	out := &qualityFixPlanV1{
		SchemaVersion:     qualityFixPlanSchemaVersion,
		RunID:             runID,
		TenantID:          report.TenantID,
		AsOf:              report.AsOf,
		SourceReportRunID: report.RunID,
		CreatedAt:         time.Now().UTC(),
		BatchRequest: qualityBatchRequest{
			DryRun:        true,
			EffectiveDate: qualityAsOfDateString(report.AsOf),
			Commands:      []qualityBatchCommand{},
		},
		Maps: qualityFixPlanMaps{
			IssueToCommandIndexes: map[string][]int{},
		},
	}

	for _, iss := range report.Issues {
		if iss.Autofix == nil || !iss.Autofix.Supported {
			continue
		}
		if strings.TrimSpace(iss.Autofix.FixKind) != fixKindAssignmentCorrect {
			continue
		}
		if iss.RuleID != ruleAssignmentSubjectMapping {
			continue
		}

		pernrTrim, ok := iss.Details["pernr_trim"].(string)
		if !ok || strings.TrimSpace(pernrTrim) == "" {
			return nil, withCode(exitValidation, fmt.Errorf("issue %s missing details.pernr_trim", iss.IssueID))
		}
		expectedSIDStr, ok := iss.Details["expected_subject_id"].(string)
		if !ok || strings.TrimSpace(expectedSIDStr) == "" {
			return nil, withCode(exitValidation, fmt.Errorf("issue %s missing details.expected_subject_id", iss.IssueID))
		}
		expectedSID, err := uuid.Parse(strings.TrimSpace(expectedSIDStr))
		if err != nil {
			return nil, withCode(exitValidation, fmt.Errorf("issue %s invalid details.expected_subject_id", iss.IssueID))
		}
		positionIDStr, ok := iss.Details["position_id"].(string)
		if !ok || strings.TrimSpace(positionIDStr) == "" {
			return nil, withCode(exitValidation, fmt.Errorf("issue %s missing details.position_id", iss.IssueID))
		}
		positionID, err := uuid.Parse(strings.TrimSpace(positionIDStr))
		if err != nil {
			return nil, withCode(exitValidation, fmt.Errorf("issue %s invalid details.position_id", iss.IssueID))
		}

		payload := struct {
			ID         uuid.UUID  `json:"id"`
			Pernr      *string    `json:"pernr"`
			SubjectID  *uuid.UUID `json:"subject_id"`
			PositionID *uuid.UUID `json:"position_id"`
		}{
			ID:         iss.Entity.ID,
			Pernr:      ptrString(strings.TrimSpace(pernrTrim)),
			SubjectID:  &expectedSID,
			PositionID: &positionID,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, withCode(exitValidation, fmt.Errorf("marshal payload: %w", err))
		}

		idx := len(out.BatchRequest.Commands)
		out.BatchRequest.Commands = append(out.BatchRequest.Commands, qualityBatchCommand{
			Type:    fixKindAssignmentCorrect,
			Payload: raw,
		})
		out.Maps.IssueToCommandIndexes[iss.IssueID.String()] = append(out.Maps.IssueToCommandIndexes[iss.IssueID.String()], idx)

		if len(out.BatchRequest.Commands) > maxCommands {
			return nil, withCode(exitValidation, fmt.Errorf("fix plan too large: %d commands > max_commands=%d", len(out.BatchRequest.Commands), maxCommands))
		}
	}

	if len(out.BatchRequest.Commands) == 0 {
		return out, errNoAutofixIssues
	}
	if err := out.validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func ptrString(v string) *string {
	v = strings.TrimSpace(v)
	return &v
}

func resolveOutputFilePath(outputPath, suggestedName string) (string, error) {
	outputPath = stringsTrim(outputPath)
	if outputPath == "" {
		return "", withCode(exitUsage, fmt.Errorf("--output is required"))
	}
	info, err := os.Stat(outputPath)
	if err == nil && info.IsDir() {
		return filepath.Join(outputPath, filepath.Base(suggestedName)), nil
	}
	if strings.HasSuffix(outputPath, string(os.PathSeparator)) {
		return filepath.Join(outputPath, filepath.Base(suggestedName)), nil
	}
	return outputPath, nil
}
