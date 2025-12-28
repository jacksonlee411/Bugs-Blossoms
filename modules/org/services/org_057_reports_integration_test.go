package services_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	orgsvc "github.com/iota-uz/iota-sdk/modules/org/services"
)

func TestOrg057StaffingSummary_ComputesTotalsAndBreakdown(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	p1, err := svc.CreatePosition(ctx, tenantID, "req-057-p1", initiatorID, orgsvc.CreatePositionInput{
		Code:               "P-001",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)
	p2, err := svc.CreatePosition(ctx, tenantID, "req-057-p2", initiatorID, orgsvc.CreatePositionInput{
		Code:               "P-002",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        2.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE org_position_slices SET position_type='regular' WHERE tenant_id=$1 AND position_id=$2`, tenantID, p1.PositionID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE org_position_slices SET position_type='contractor' WHERE tenant_id=$1 AND position_id=$2`, tenantID, p2.PositionID)
	require.NoError(t, err)

	asOfReport := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	subjectID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO org_assignments (
			tenant_id, position_id, subject_type, subject_id, pernr, assignment_type, is_primary, allocated_fte, effective_date, end_date
		)
		VALUES (
			$1,$2,'person',$3,'pernr-057-1','primary',true,1.0,
			($4 AT TIME ZONE 'UTC')::date,
			($5 AT TIME ZONE 'UTC')::date
		)
		`, tenantID, p2.PositionID, subjectID, time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	res, err := svc.GetStaffingSummary(ctx, tenantID, orgsvc.StaffingSummaryInput{
		OrgNodeID:     &rootNodeID,
		EffectiveDate: asOfReport,
		Scope:         orgsvc.StaffingScopeSubtree,
		GroupBy:       orgsvc.StaffingGroupByPositionType,
	})
	require.NoError(t, err)

	require.Equal(t, 2, res.Totals.PositionsTotal)
	require.InEpsilon(t, 3.0, res.Totals.CapacityFTE, 0.0001)
	require.InEpsilon(t, 1.0, res.Totals.OccupiedFTE, 0.0001)
	require.InEpsilon(t, 2.0, res.Totals.AvailableFTE, 0.0001)
	require.InEpsilon(t, 1.0/3.0, res.Totals.FillRate, 0.0001)

	require.Len(t, res.Breakdown, 2)
	require.Equal(t, "contractor", res.Breakdown[0].Key)
	require.Equal(t, "regular", res.Breakdown[1].Key)
}

func TestOrg057StaffingVacancies_ComputesVacancySince(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-057-vac", initiatorID, orgsvc.CreatePositionInput{
		Code:               "P-VAC",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE org_position_slices SET position_type='regular' WHERE tenant_id=$1 AND position_id=$2`, tenantID, pos.PositionID)
	require.NoError(t, err)

	subjectID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO org_assignments (
			tenant_id, position_id, subject_type, subject_id, pernr, assignment_type, is_primary, allocated_fte, effective_date, end_date
		)
		VALUES (
			$1,$2,'person',$3,'pernr-057-2','primary',true,1.0,
			($4 AT TIME ZONE 'UTC')::date,
			($5 AT TIME ZONE 'UTC')::date
		)
		`, tenantID, pos.PositionID, subjectID, time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC), time.Date(2025, 2, 9, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	asOfReport := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	res, err := svc.ListStaffingVacancies(ctx, tenantID, orgsvc.StaffingVacanciesInput{
		OrgNodeID:     &rootNodeID,
		EffectiveDate: asOfReport,
		Scope:         orgsvc.StaffingScopeSelf,
		Limit:         10,
	})
	require.NoError(t, err)

	require.Len(t, res.Items, 1)
	item := res.Items[0]
	require.Equal(t, pos.PositionID, item.PositionID)
	require.True(t, item.VacancySince.Equal(time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, 19, item.VacancyAgeDays)
	require.Equal(t, "regular", item.PositionType)
}

func TestOrg057StaffingTimeToFill_ComputesSummaryAndBreakdown(t *testing.T) {
	ctx, pool, tenantID, rootNodeID, asOf, svc := setupOrg053DB(t)

	initiatorID := uuid.New()
	pos, err := svc.CreatePosition(ctx, tenantID, "req-057-ttf", initiatorID, orgsvc.CreatePositionInput{
		Code:               "P-TTF",
		OrgNodeID:          rootNodeID,
		EffectiveDate:      asOf,
		PositionType:       "regular",
		EmploymentType:     "full_time",
		JobFamilyGroupCode: "TST",
		JobFamilyCode:      "TST-FAMILY",
		JobRoleCode:        "TST-ROLE",
		JobLevelCode:       "L1",
		CapacityFTE:        1.0,
		ReasonCode:         "create",
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE org_position_slices SET position_type='regular' WHERE tenant_id=$1 AND position_id=$2`, tenantID, pos.PositionID)
	require.NoError(t, err)

	subjectID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO org_assignments (
			tenant_id, position_id, subject_type, subject_id, pernr, assignment_type, is_primary, allocated_fte, effective_date, end_date
		)
		VALUES
			(
				$1,$2,'person',$3,'pernr-057-3','primary',true,1.0,
				($4 AT TIME ZONE 'UTC')::date,
				($5 AT TIME ZONE 'UTC')::date
			),
			(
				$1,$2,'person',$3,'pernr-057-3','primary',true,1.0,
				($6 AT TIME ZONE 'UTC')::date,
				($7 AT TIME ZONE 'UTC')::date
			)
		`, tenantID,
		pos.PositionID,
		subjectID,
		time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 2, 9, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 2, 20, 0, 0, 0, 0, time.UTC),
		time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	res, err := svc.GetStaffingTimeToFill(ctx, tenantID, orgsvc.StaffingTimeToFillInput{
		OrgNodeID: &rootNodeID,
		From:      time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		To:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Scope:     orgsvc.StaffingScopeSubtree,
		GroupBy:   orgsvc.StaffingGroupByPositionType,
	})
	require.NoError(t, err)

	require.Equal(t, 1, res.Summary.FilledCount)
	require.InEpsilon(t, 10.0, res.Summary.AvgDays, 0.0001)
	require.Equal(t, 10, res.Summary.P50Days)
	require.Equal(t, 10, res.Summary.P95Days)

	require.Len(t, res.Breakdown, 1)
	require.Equal(t, "regular", res.Breakdown[0].Key)
	require.Equal(t, 1, res.Breakdown[0].FilledCount)
	require.InEpsilon(t, 10.0, res.Breakdown[0].AvgDays, 0.0001)
}
