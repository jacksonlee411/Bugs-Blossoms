package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAutoPositionID_IsDeterministic(t *testing.T) {
	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	orgNodeID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	subjectID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

	a, err := autoPositionID(tenantID, orgNodeID, subjectID)
	require.NoError(t, err)

	b, err := autoPositionID(tenantID, orgNodeID, subjectID)
	require.NoError(t, err)

	require.Equal(t, a, b)
	require.NotEqual(t, uuid.Nil, a)
}

func TestAutoPositionCode_HasPrefixAndIsUpper(t *testing.T) {
	id := uuid.MustParse("2ee72897-775c-49eb-94a2-1d6b9e157701")
	code := autoPositionCode(id)
	require.Equal(t, "AUTO-2EE72897775C49EB", code)
}

func TestCreateAssignment_RejectsDisabledAssignmentType(t *testing.T) {
	svc := NewOrgService(nil)
	tenantID := uuid.New()
	_, err := svc.CreateAssignment(
		context.Background(),
		tenantID,
		"req-1",
		uuid.New(),
		CreateAssignmentInput{
			Pernr:          "000123",
			EffectiveDate:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			AssignmentType: "matrix",
			PositionID:     nil,
			OrgNodeID:      nil,
		},
	)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_ASSIGNMENT_TYPE_DISABLED", svcErr.Code)
}

func TestCreateAssignment_RejectsSubjectMismatch(t *testing.T) {
	svc := NewOrgService(nil)
	tenantID := uuid.New()
	wrongSubjectID := uuid.New()
	_, err := svc.CreateAssignment(
		context.Background(),
		tenantID,
		"req-1",
		uuid.New(),
		CreateAssignmentInput{
			Pernr:         "000123",
			EffectiveDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			PositionID:    nil,
			OrgNodeID:     nil,
			SubjectID:     &wrongSubjectID,
		},
	)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 422, svcErr.Status)
	require.Equal(t, "ORG_SUBJECT_MISMATCH", svcErr.Code)
}

func TestCreateNode_RejectsMissingFields(t *testing.T) {
	svc := NewOrgService(nil)
	tenantID := uuid.New()
	_, err := svc.CreateNode(
		context.Background(),
		tenantID,
		"req-1",
		uuid.New(),
		CreateNodeInput{},
	)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	require.Equal(t, 400, svcErr.Status)
	require.Equal(t, "ORG_INVALID_BODY", svcErr.Code)
}
