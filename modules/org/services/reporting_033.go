package services

import "github.com/google/uuid"

type OrgReportingBuildResult struct {
	TenantID               uuid.UUID
	HierarchyType          string
	AsOfDate               string
	SnapshotBuildID        uuid.UUID
	DryRun                 bool
	IncludedSecurityGroups bool
	IncludedLinks          bool
	RowCount               int64
}
