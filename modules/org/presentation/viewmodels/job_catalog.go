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

type JobRoleRow struct {
	ID          uuid.UUID
	JobFamilyID uuid.UUID
	Code        string
	Name        string
	IsActive    bool
}

type JobLevelRow struct {
	ID           uuid.UUID
	JobRoleID    uuid.UUID
	Code         string
	Name         string
	DisplayOrder int
	IsActive     bool
}
