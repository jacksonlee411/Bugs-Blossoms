package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SnapshotItem struct {
	EntityType string          `json:"entity_type"`
	EntityID   uuid.UUID       `json:"entity_id"`
	NewValues  json.RawMessage `json:"new_values"`
}

type SnapshotResult struct {
	TenantID      uuid.UUID      `json:"tenant_id"`
	EffectiveDate time.Time      `json:"effective_date"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Includes      []string       `json:"includes"`
	Limit         int            `json:"limit"`
	Items         []SnapshotItem `json:"items"`
	NextCursor    *string        `json:"next_cursor"`
}

var snapshotTypeOrder = []string{"nodes", "edges", "positions", "assignments"}

func (s *OrgService) GetSnapshot(ctx context.Context, tenantID uuid.UUID, asOf time.Time, includes []string, limit int, cursor string) (*SnapshotResult, error) {
	if tenantID == uuid.Nil {
		return nil, newServiceError(400, "ORG_NO_TENANT", "tenant_id is required", nil)
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	asOf = asOf.UTC()

	if len(includes) == 0 {
		includes = []string{"nodes", "edges"}
	}
	normalizedIncludes := make([]string, 0, len(includes))
	includeSet := map[string]bool{}
	for _, v := range includes {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			continue
		}
		if v != "nodes" && v != "edges" && v != "positions" && v != "assignments" {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "include is invalid", nil)
		}
		if includeSet[v] {
			continue
		}
		includeSet[v] = true
		normalizedIncludes = append(normalizedIncludes, v)
	}
	if len(normalizedIncludes) == 0 {
		return nil, newServiceError(400, "ORG_INVALID_QUERY", "include is invalid", nil)
	}

	if limit == 0 {
		limit = 2000
	}
	if limit < 1 || limit > 10000 {
		return nil, newServiceError(400, "ORG_INVALID_QUERY", "limit is invalid", nil)
	}

	startEntityType := ""
	var startAfterID *uuid.UUID
	if strings.TrimSpace(cursor) != "" {
		parts := strings.SplitN(cursor, ":", 2)
		if len(parts) != 2 {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "cursor is invalid", nil)
		}
		startEntityType = strings.TrimSpace(parts[0])
		id, err := uuid.Parse(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "cursor is invalid", nil)
		}
		startAfterID = &id
		if startEntityType != "org_node" && startEntityType != "org_edge" && startEntityType != "org_position" && startEntityType != "org_assignment" {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "cursor is invalid", nil)
		}

		if !includeSet[mapEntityTypeToInclude(startEntityType)] {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "cursor is invalid", nil)
		}
	}

	typeToFetcher := map[string]func(ctx context.Context, afterID *uuid.UUID, n int) ([]SnapshotItem, error){
		"nodes": func(ctx context.Context, afterID *uuid.UUID, n int) ([]SnapshotItem, error) {
			return s.repo.ListSnapshotNodes(ctx, tenantID, asOf, afterID, n)
		},
		"edges": func(ctx context.Context, afterID *uuid.UUID, n int) ([]SnapshotItem, error) {
			return s.repo.ListSnapshotEdges(ctx, tenantID, asOf, afterID, n)
		},
		"positions": func(ctx context.Context, afterID *uuid.UUID, n int) ([]SnapshotItem, error) {
			return s.repo.ListSnapshotPositions(ctx, tenantID, asOf, afterID, n)
		},
		"assignments": func(ctx context.Context, afterID *uuid.UUID, n int) ([]SnapshotItem, error) {
			return s.repo.ListSnapshotAssignments(ctx, tenantID, asOf, afterID, n)
		},
	}

	order := make([]string, 0, len(snapshotTypeOrder))
	for _, t := range snapshotTypeOrder {
		if includeSet[t] {
			order = append(order, t)
		}
	}

	startIndex := 0
	if startEntityType != "" {
		want := mapEntityTypeToInclude(startEntityType)
		found := false
		for i, t := range order {
			if t == want {
				startIndex = i
				found = true
				break
			}
		}
		if !found {
			return nil, newServiceError(400, "ORG_INVALID_QUERY", "cursor is invalid", nil)
		}
	}

	items := make([]SnapshotItem, 0, minInt(limit, 256))
	var nextCursor *string

	for i := startIndex; i < len(order) && len(items) < limit; i++ {
		t := order[i]
		afterID := (*uuid.UUID)(nil)
		if i == startIndex && startAfterID != nil {
			afterID = startAfterID
		}

		remaining := limit - len(items)
		rows, err := typeToFetcher[t](ctx, afterID, remaining+1)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}

		hasMore := len(rows) > remaining
		if hasMore {
			rows = rows[:remaining]
		}
		items = append(items, rows...)

		if hasMore {
			last := items[len(items)-1]
			c := fmt.Sprintf("%s:%s", last.EntityType, last.EntityID.String())
			nextCursor = &c
			break
		}

		// If we filled the page exactly at a type boundary, emit a cursor that
		// advances to the next entity type (starting from the beginning).
		if len(items) == limit && i+1 < len(order) {
			nextType := includeToEntityType(order[i+1])
			if nextType != "" {
				c := fmt.Sprintf("%s:%s", nextType, uuid.Nil.String())
				nextCursor = &c
			}
			break
		}
	}

	return &SnapshotResult{
		TenantID:      tenantID,
		EffectiveDate: asOf,
		GeneratedAt:   time.Now().UTC(),
		Includes:      order,
		Limit:         limit,
		Items:         items,
		NextCursor:    nextCursor,
	}, nil
}

func mapEntityTypeToInclude(entityType string) string {
	switch entityType {
	case "org_node":
		return "nodes"
	case "org_edge":
		return "edges"
	case "org_position":
		return "positions"
	case "org_assignment":
		return "assignments"
	default:
		return ""
	}
}

func includeToEntityType(include string) string {
	switch include {
	case "nodes":
		return "org_node"
	case "edges":
		return "org_edge"
	case "positions":
		return "org_position"
	case "assignments":
		return "org_assignment"
	default:
		return ""
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
