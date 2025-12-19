package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateFixPlanFromReport_AssignmentCorrect(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	report := &qualityReportV1{
		SchemaVersion: qualityReportSchemaVersion,
		RunID:         uuid.New(),
		TenantID:      tenantID,
		AsOf:          asOf,
		GeneratedAt:   asOf,
		Ruleset:       qualityRuleset{Name: qualityRulesetName, Version: qualityRulesetVersion},
		Issues: []qualityIssue{
			{
				IssueID:  uuid.New(),
				RuleID:   ruleAssignmentSubjectMapping,
				Severity: severityError,
				Entity:   qualityEntityRef{Type: "org_assignment", ID: uuid.New()},
				Message:  "subject_id mismatch with SSOT mapping",
				Details: map[string]any{
					"pernr_trim":          "000123",
					"expected_subject_id": uuid.New().String(),
					"position_id":         uuid.New().String(),
				},
				Autofix: &qualityAutofix{Supported: true, FixKind: fixKindAssignmentCorrect, Risk: "low"},
			},
		},
	}

	fixPlan, err := generateFixPlanFromReport(report, 10)
	if err != nil {
		t.Fatalf("generateFixPlanFromReport: %v", err)
	}
	if fixPlan.TenantID != tenantID {
		t.Fatalf("tenant mismatch")
	}
	if fixPlan.AsOf != asOf {
		t.Fatalf("as_of mismatch")
	}
	if !fixPlan.BatchRequest.DryRun {
		t.Fatalf("expected dry_run=true in fix plan")
	}
	if fixPlan.BatchRequest.EffectiveDate != "2025-03-01" {
		t.Fatalf("effective_date mismatch: %s", fixPlan.BatchRequest.EffectiveDate)
	}
	if len(fixPlan.BatchRequest.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(fixPlan.BatchRequest.Commands))
	}
	cmd := fixPlan.BatchRequest.Commands[0]
	if cmd.Type != fixKindAssignmentCorrect {
		t.Fatalf("unexpected command type: %s", cmd.Type)
	}
	var payload assignmentCorrectPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("payload json invalid: %v", err)
	}
	if payload.ID == uuid.Nil || payload.Pernr == nil || payload.SubjectID == nil || payload.PositionID == nil {
		t.Fatalf("payload missing fields: %+v", payload)
	}
}

func TestGenerateFixPlanFromReport_MaxCommands(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	report := &qualityReportV1{
		SchemaVersion: qualityReportSchemaVersion,
		RunID:         uuid.New(),
		TenantID:      tenantID,
		AsOf:          asOf,
		GeneratedAt:   asOf,
		Ruleset:       qualityRuleset{Name: qualityRulesetName, Version: qualityRulesetVersion},
		Issues:        []qualityIssue{},
	}
	for i := 0; i < 2; i++ {
		report.Issues = append(report.Issues, qualityIssue{
			IssueID:  uuid.New(),
			RuleID:   ruleAssignmentSubjectMapping,
			Severity: severityError,
			Entity:   qualityEntityRef{Type: "org_assignment", ID: uuid.New()},
			Message:  "subject_id mismatch with SSOT mapping",
			Details: map[string]any{
				"pernr_trim":          "000123",
				"expected_subject_id": uuid.New().String(),
				"position_id":         uuid.New().String(),
			},
			Autofix: &qualityAutofix{Supported: true, FixKind: fixKindAssignmentCorrect, Risk: "low"},
		})
	}

	_, err := generateFixPlanFromReport(report, 1)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ce *cliError
	if !as(err, &ce) || ce.code != exitValidation {
		t.Fatalf("expected exitValidation cliError, got %T: %v", err, err)
	}
}

func TestGenerateFixPlanFromReport_NoAutofixIssues(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	report := &qualityReportV1{
		SchemaVersion: qualityReportSchemaVersion,
		RunID:         uuid.New(),
		TenantID:      tenantID,
		AsOf:          asOf,
		GeneratedAt:   asOf,
		Ruleset:       qualityRuleset{Name: qualityRulesetName, Version: qualityRulesetVersion},
		Issues:        []qualityIssue{},
	}

	fixPlan, err := generateFixPlanFromReport(report, 10)
	if !errors.Is(err, errNoAutofixIssues) {
		t.Fatalf("expected errNoAutofixIssues, got %v", err)
	}
	if fixPlan == nil {
		t.Fatalf("expected fixPlan")
	}
	if len(fixPlan.BatchRequest.Commands) != 0 {
		t.Fatalf("expected 0 commands, got %d", len(fixPlan.BatchRequest.Commands))
	}
}
