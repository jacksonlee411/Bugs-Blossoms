package services

import (
	"errors"

	"github.com/google/uuid"
)

type DeepReadBackend string

const (
	DeepReadBackendEdges    DeepReadBackend = "edges"
	DeepReadBackendClosure  DeepReadBackend = "closure"
	DeepReadBackendSnapshot DeepReadBackend = "snapshot"
)

var ErrOrgDeepReadBuildNotReady = errors.New("org deep read build is not ready")

type DeepReadRelation struct {
	NodeID uuid.UUID
	Depth  int
}

type DeepReadBuildResult struct {
	TenantID        uuid.UUID
	HierarchyType   string
	Backend         DeepReadBackend
	BuildID         uuid.UUID
	AsOfDate        string
	DryRun          bool
	Activated       bool
	RowCount        int64
	MaxDepth        int
	SourceRequestID string
}

type DeepReadPruneResult struct {
	TenantID      uuid.UUID
	HierarchyType string
	Backend       DeepReadBackend
	DeletedBuilds int
}
