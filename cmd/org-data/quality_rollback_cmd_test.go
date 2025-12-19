package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildRollbackBatchRequest(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	aID := uuid.New()
	posID := uuid.New()
	sid := uuid.New()
	manifest := qualityFixManifestV1{
		SchemaVersion:      qualityFixManSchemaVersion,
		RunID:              uuid.New(),
		TenantID:           tenantID,
		AsOf:               asOf,
		AppliedAt:          time.Now().UTC(),
		SourceFixPlanRunID: uuid.New(),
		BatchRequest:       qualityBatchRequest{DryRun: false, EffectiveDate: "2025-03-01", Commands: []qualityBatchCommand{{Type: fixKindAssignmentCorrect, Payload: json.RawMessage(`{}`)}}},
		Before: qualityFixBefore{
			Assignments: []qualityBeforeAssignment{
				{ID: aID, Pernr: "000123", SubjectID: sid, PositionID: posID},
			},
		},
		Results: qualityFixResults{Ok: true},
	}

	req, err := buildRollbackBatchRequest(manifest, true)
	if err != nil {
		t.Fatalf("buildRollbackBatchRequest: %v", err)
	}
	if !req.DryRun {
		t.Fatalf("expected dry_run=true")
	}
	if req.EffectiveDate != "2025-03-01" {
		t.Fatalf("effective_date mismatch: %s", req.EffectiveDate)
	}
	if len(req.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(req.Commands))
	}
	var payload assignmentCorrectPayload
	if err := json.Unmarshal(req.Commands[0].Payload, &payload); err != nil {
		t.Fatalf("payload json invalid: %v", err)
	}
	if payload.ID != aID {
		t.Fatalf("id mismatch")
	}
	if payload.Pernr == nil || *payload.Pernr != "000123" {
		t.Fatalf("pernr mismatch")
	}
}

func TestEnsureChangeRequestPayloadMatchesFixPlan_IgnoresDryRun(t *testing.T) {
	t.Parallel()

	planReq := qualityBatchRequest{
		DryRun:        true,
		EffectiveDate: "2025-03-01",
		Commands: []qualityBatchCommand{
			{Type: fixKindAssignmentCorrect, Payload: json.RawMessage(`{"id":"00000000-0000-0000-0000-000000000001","pernr":"000123","subject_id":"00000000-0000-0000-0000-000000000002","position_id":"00000000-0000-0000-0000-000000000003"}`)},
		},
	}

	// change request payload omits dry_run (030 payload is batch-like but doesn't have to carry dry_run)
	crPayload := json.RawMessage(`{"effective_date":"2025-03-01","commands":[{"type":"assignment.correct","payload":{"id":"00000000-0000-0000-0000-000000000001","pernr":"000123","subject_id":"00000000-0000-0000-0000-000000000002","position_id":"00000000-0000-0000-0000-000000000003"}}]}`)
	if err := ensureChangeRequestPayloadMatchesFixPlan(crPayload, planReq); err != nil {
		t.Fatalf("expected match, got: %v", err)
	}
}
