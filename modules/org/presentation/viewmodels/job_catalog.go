package viewmodels

import "github.com/google/uuid"

type JobFamilyGroupRow struct {
	ID       uuid.UUID
	Code     string
	Name     string
	IsActive bool
}

type JobFamilyRow struct {
	ID               uuid.UUID
	JobFamilyGroupID uuid.UUID
	Code             string
	Name             string
	IsActive         bool
}

type JobLevelRow struct {
	ID           uuid.UUID
	Code         string
	Name         string
	DisplayOrder int
	IsActive     bool
}

type JobProfileJobFamilyRow struct {
	JobFamilyID        uuid.UUID
	JobFamilyGroupCode string
	JobFamilyCode      string
	JobFamilyName      string
	IsPrimary          bool
}

type JobProfileRow struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Description string
	IsActive    bool
	JobFamilies []JobProfileJobFamilyRow
}
