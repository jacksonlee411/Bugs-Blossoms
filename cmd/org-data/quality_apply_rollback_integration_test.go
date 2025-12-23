package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOrgData_QualityApplyAndRollback_UsesBatchAndManifest(t *testing.T) {
	t.Setenv("ORG_DATA_QUALITY_ENABLED", "true")

	tenantID := uuid.New()
	asOf := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	assignmentID := uuid.New()
	positionID := uuid.New()
	subjectID := uuid.New()

	var mu sync.Mutex
	var batchRequests []qualityBatchRequest
	snapshotCalls := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/org/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		snapshotCalls++
		mu.Unlock()

		values := snapshotAssignmentValues{
			OrgAssignmentID: assignmentID,
			PositionID:      positionID,
			SubjectType:     "person",
			SubjectID:       subjectID,
			Pernr:           "0001",
			AssignmentType:  "primary",
			AllocatedFTE:    1.0,
			EffectiveDate:   asOf,
			EndDate:         maxEndDate,
		}
		raw, err := json.Marshal(values)
		if err != nil {
			t.Errorf("marshal snapshotAssignmentValues: %v", err)
			http.Error(w, "marshal failed", http.StatusInternalServerError)
			return
		}

		res := snapshotResult{
			TenantID:      tenantID,
			EffectiveDate: asOf,
			GeneratedAt:   time.Now().UTC(),
			Includes:      []string{"assignments"},
			Limit:         10000,
			Items: []snapshotItem{
				{
					EntityType: "org_assignment",
					EntityID:   assignmentID,
					NewValues:  json.RawMessage(raw),
				},
			},
			NextCursor: nil,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			t.Errorf("encode snapshot response: %v", err)
		}
	})
	mux.HandleFunc("/org/api/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read batch request: %v", err)
			http.Error(w, "read failed", http.StatusInternalServerError)
			return
		}
		_ = r.Body.Close()

		var req qualityBatchRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Errorf("decode batch request: %v", err)
			http.Error(w, "decode failed", http.StatusBadRequest)
			return
		}

		mu.Lock()
		batchRequests = append(batchRequests, req)
		mu.Unlock()

		resp := struct {
			DryRun         bool                        `json:"dry_run"`
			Results        []qualityBatchCommandResult `json:"results"`
			EventsEnqueued int                         `json:"events_enqueued"`
		}{
			DryRun: req.DryRun,
			Results: []qualityBatchCommandResult{
				{Index: 0, Type: fixKindAssignmentCorrect, Ok: true, Result: map[string]any{"status": "ok"}},
			},
			EventsEnqueued: 0,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode batch response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	outDir := t.TempDir()
	fixPlanPath := filepath.Join(outDir, "org_quality_fix_plan.v1.json")

	payload, err := json.Marshal(assignmentCorrectPayload{
		ID:         assignmentID,
		Pernr:      ptr("0001"),
		SubjectID:  &subjectID,
		PositionID: &positionID,
	})
	require.NoError(t, err)

	fixPlan := qualityFixPlanV1{
		SchemaVersion:     qualityFixPlanSchemaVersion,
		RunID:             uuid.New(),
		TenantID:          tenantID,
		AsOf:              asOf,
		SourceReportRunID: uuid.New(),
		CreatedAt:         time.Now().UTC(),
		BatchRequest: qualityBatchRequest{
			DryRun:        true,
			EffectiveDate: "2025-01-01",
			Commands: []qualityBatchCommand{
				{Type: fixKindAssignmentCorrect, Payload: payload},
			},
		},
		Maps: qualityFixPlanMaps{IssueToCommandIndexes: map[string][]int{}},
	}
	require.NoError(t, writeJSONFile(fixPlanPath, &fixPlan))

	applyCmd := newQualityApplyCmd()
	applyCmd.SetArgs([]string{
		"--fix-plan", fixPlanPath,
		"--output", outDir,
		"--base-url", srv.URL,
		"--auth-token", "test",
	})
	require.NoError(t, applyCmd.Execute())

	manifestPath := singleGlob(t, filepath.Join(outDir, "org_quality_fix_manifest_*.json"))

	var manifest qualityFixManifestV1
	require.NoError(t, readJSONFile(manifestPath, &manifest))
	require.NoError(t, manifest.validate())
	require.Equal(t, tenantID, manifest.TenantID)
	require.True(t, manifest.BatchRequest.DryRun)
	require.Len(t, manifest.Before.Assignments, 1)
	require.Equal(t, assignmentID, manifest.Before.Assignments[0].ID)

	rollbackCmd := newQualityRollbackCmd()
	rollbackCmd.SetArgs([]string{
		"--manifest", manifestPath,
		"--base-url", srv.URL,
		"--auth-token", "test",
		"--apply",
		"--yes",
	})
	require.NoError(t, rollbackCmd.Execute())

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, snapshotCalls)
	require.Len(t, batchRequests, 2)

	// Apply: dry-run, rollback: apply.
	require.True(t, batchRequests[0].DryRun)
	require.False(t, batchRequests[1].DryRun)

	// Rollback payload should be filled from manifest.before.assignments.
	require.Len(t, batchRequests[1].Commands, 1)
	var rollbackPayload assignmentCorrectPayload
	require.NoError(t, json.Unmarshal(batchRequests[1].Commands[0].Payload, &rollbackPayload))
	require.Equal(t, assignmentID, rollbackPayload.ID)
	require.NotNil(t, rollbackPayload.Pernr)
	require.Equal(t, "0001", *rollbackPayload.Pernr)
	require.NotNil(t, rollbackPayload.SubjectID)
	require.Equal(t, subjectID, *rollbackPayload.SubjectID)
	require.NotNil(t, rollbackPayload.PositionID)
	require.Equal(t, positionID, *rollbackPayload.PositionID)
}

func singleGlob(tb testing.TB, pattern string) string {
	tb.Helper()
	matches, err := filepath.Glob(pattern)
	require.NoError(tb, err)
	require.Len(tb, matches, 1)
	return matches[0]
}

func ptr[T any](v T) *T {
	return &v
}
