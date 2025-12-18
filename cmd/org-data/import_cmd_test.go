package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeNodes_EndDateAutoFill(t *testing.T) {
	runID := uuid.New()
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	startedAt := time.Now().UTC()

	nodes := []nodeCSVRow{
		{
			line:          2,
			code:          "ROOT",
			nodeType:      "OrgUnit",
			name:          "Root",
			i18nNames:     []byte(`{}`),
			status:        "active",
			parentCode:    nil,
			effectiveDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			line:          3,
			code:          "ROOT",
			nodeType:      "OrgUnit",
			name:          "Root v2",
			i18nNames:     []byte(`{}`),
			status:        "active",
			parentCode:    nil,
			effectiveDate: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	data, err := normalizeAndValidate(runID, tenantID, startedAt, nodes, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// first slice end_date should auto-fill to next effective_date
	if got := data.nodeSlices[0].endDate; !got.Equal(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected end_date: %s", got.Format(time.RFC3339))
	}
	// last slice end_date should be maxEndDate
	if got := data.nodeSlices[1].endDate; !got.Equal(maxEndDate) {
		t.Fatalf("unexpected end_date: %s", got.Format(time.RFC3339))
	}
}

func TestNormalizeNodes_DetectsCycle(t *testing.T) {
	runID := uuid.New()
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	startedAt := time.Now().UTC()

	aParent := "B"
	bParent := "A"
	nodes := []nodeCSVRow{
		{
			line:            2,
			code:            "A",
			nodeType:        "OrgUnit",
			name:            "A",
			i18nNames:       []byte(`{}`),
			status:          "active",
			parentCode:      &aParent,
			effectiveDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:         maxEndDate,
			endDateProvided: true,
		},
		{
			line:            3,
			code:            "B",
			nodeType:        "OrgUnit",
			name:            "B",
			i18nNames:       []byte(`{}`),
			status:          "active",
			parentCode:      &bParent,
			effectiveDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:         maxEndDate,
			endDateProvided: true,
		},
	}

	_, err := normalizeAndValidate(runID, tenantID, startedAt, nodes, nil, nil, false)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestNormalizeAssignments_SubjectIDMismatch(t *testing.T) {
	runID := uuid.New()
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	startedAt := time.Now().UTC()

	rootParent := (*string)(nil)
	nodes := []nodeCSVRow{
		{
			line:            2,
			code:            "ROOT",
			nodeType:        "OrgUnit",
			name:            "Root",
			i18nNames:       []byte(`{}`),
			status:          "active",
			parentCode:      rootParent,
			effectiveDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:         maxEndDate,
			endDateProvided: true,
		},
	}

	positions := []positionCSVRow{
		{
			line:            2,
			code:            "P1",
			orgNodeCode:     "ROOT",
			status:          "active",
			isAutoCreated:   false,
			effectiveDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:         maxEndDate,
			endDateProvided: true,
		},
	}

	bad := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	assignments := []assignmentCSVRow{
		{
			line:            2,
			positionCode:    "P1",
			assignmentType:  "primary",
			pernr:           "0001",
			subjectID:       &bad,
			effectiveDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endDate:         maxEndDate,
			endDateProvided: true,
		},
	}

	_, err := normalizeAndValidate(runID, tenantID, startedAt, nodes, positions, assignments, false)
	if err == nil {
		t.Fatalf("expected error")
	}
}
