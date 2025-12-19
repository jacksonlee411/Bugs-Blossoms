package controllers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/modules/org/services"
)

type orgHealthStatus string

const (
	orgHealthStatusHealthy  orgHealthStatus = "healthy"
	orgHealthStatusDegraded orgHealthStatus = "degraded"
	orgHealthStatusDown     orgHealthStatus = "down"
)

type orgHealthResponse struct {
	Status    orgHealthStatus `json:"status"`
	Timestamp string          `json:"timestamp"`
	Checks    map[string]any  `json:"checks"`
}

type orgComponentHealth struct {
	Status       orgHealthStatus `json:"status"`
	ResponseTime string          `json:"responseTime,omitempty"`
	Error        string          `json:"error,omitempty"`
	Details      map[string]any  `json:"details,omitempty"`
}

const (
	orgOutboxPendingDegradedThreshold = int64(1000)
	orgOutboxOldestAvailableDegraded  = 5 * time.Minute
	orgDeepReadBuildAgeDegraded       = 24 * time.Hour
	orgDBDegradedLatency              = 100 * time.Millisecond
)

func (c *OrgAPIController) GetOpsHealth(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgOpsAuthzObject, "admin") {
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	response := c.performOrgOpsHealthChecks(r.Context(), tenantID, "OrgUnit", asOf)

	var status int
	switch response.Status {
	case orgHealthStatusHealthy, orgHealthStatusDegraded:
		status = http.StatusOK
	case orgHealthStatusDown:
		status = http.StatusServiceUnavailable
	default:
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, response)
}

func (c *OrgAPIController) performOrgOpsHealthChecks(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) orgHealthResponse {
	checks := make(map[string]any)
	overall := orgHealthStatusHealthy

	dbHealth := c.checkDatabase(ctx)
	checks["database"] = dbHealth
	overall = mergeOrgHealthStatus(overall, dbHealth.Status)

	outboxHealth := c.checkOrgOutbox(ctx, tenantID)
	checks["outbox"] = outboxHealth
	overall = mergeOrgHealthStatus(overall, outboxHealth.Status)

	deepRead := c.checkOrgDeepRead(ctx, tenantID, hierarchyType, asOf)
	checks["deep_read"] = deepRead
	overall = mergeOrgHealthStatus(overall, deepRead.Status)

	checks["cache"] = orgComponentHealth{
		Status: orgHealthStatusHealthy,
		Details: map[string]any{
			"enabled": services.OrgCacheEnabled(),
		},
	}

	return orgHealthResponse{
		Status:    overall,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}
}

func mergeOrgHealthStatus(current, next orgHealthStatus) orgHealthStatus {
	if next == orgHealthStatusDown {
		return orgHealthStatusDown
	}
	if next == orgHealthStatusDegraded && current == orgHealthStatusHealthy {
		return orgHealthStatusDegraded
	}
	return current
}

func (c *OrgAPIController) checkDatabase(ctx context.Context) orgComponentHealth {
	start := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := c.app.DB()
	if db == nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        "database connection pool not available",
		}
	}

	var result int
	err := db.QueryRow(timeoutCtx, "SELECT 1").Scan(&result)
	responseTime := time.Since(start)
	if err != nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: responseTime.String(),
			Error:        fmt.Sprintf("database query failed: %v", err),
		}
	}

	status := orgHealthStatusHealthy
	if responseTime > orgDBDegradedLatency {
		status = orgHealthStatusDegraded
	}

	return orgComponentHealth{
		Status:       status,
		ResponseTime: responseTime.String(),
	}
}

func (c *OrgAPIController) checkOrgOutbox(ctx context.Context, tenantID uuid.UUID) orgComponentHealth {
	start := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := c.app.DB()
	if db == nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        "database connection pool not available",
		}
	}

	var pending int64
	if err := db.QueryRow(timeoutCtx, `SELECT count(*) FROM org_outbox WHERE published_at IS NULL AND tenant_id = $1`, tenantID).Scan(&pending); err != nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        fmt.Sprintf("outbox pending query failed: %v", err),
		}
	}

	var locked int64
	if err := db.QueryRow(timeoutCtx, `SELECT count(*) FROM org_outbox WHERE published_at IS NULL AND locked_at IS NOT NULL AND tenant_id = $1`, tenantID).Scan(&locked); err != nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        fmt.Sprintf("outbox locked query failed: %v", err),
		}
	}

	var oldestAvailable *time.Time
	if err := db.QueryRow(timeoutCtx, `SELECT min(available_at) FROM org_outbox WHERE published_at IS NULL AND tenant_id = $1`, tenantID).Scan(&oldestAvailable); err != nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        fmt.Sprintf("outbox oldest query failed: %v", err),
		}
	}

	status := orgHealthStatusHealthy
	var oldestAgeStr string
	if oldestAvailable != nil {
		age := time.Since(*oldestAvailable)
		oldestAgeStr = age.Truncate(time.Second).String()
		if age > orgOutboxOldestAvailableDegraded {
			status = orgHealthStatusDegraded
		}
	}
	if pending > orgOutboxPendingDegradedThreshold {
		status = orgHealthStatusDegraded
	}

	details := map[string]any{
		"pending": pending,
		"locked":  locked,
	}
	if oldestAgeStr != "" {
		details["oldest_available_age"] = oldestAgeStr
	}

	return orgComponentHealth{
		Status:       status,
		ResponseTime: time.Since(start).String(),
		Details:      details,
	}
}

func (c *OrgAPIController) checkOrgDeepRead(ctx context.Context, tenantID uuid.UUID, hierarchyType string, asOf time.Time) orgComponentHealth {
	start := time.Now()

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := c.app.DB()
	if db == nil {
		return orgComponentHealth{
			Status:       orgHealthStatusDown,
			ResponseTime: time.Since(start).String(),
			Error:        "database connection pool not available",
		}
	}

	backend := services.OrgDeepReadBackendForTenant(tenantID)
	services.RecordDeepReadBackendMetric(backend)

	switch backend {
	case services.DeepReadBackendEdges:
		return orgComponentHealth{
			Status:       orgHealthStatusHealthy,
			ResponseTime: time.Since(start).String(),
			Details: map[string]any{
				"backend": string(backend),
			},
		}
	case services.DeepReadBackendClosure:
		var builtAt time.Time
		var buildStatus string
		if err := db.QueryRow(
			timeoutCtx,
			`SELECT built_at, status FROM org_hierarchy_closure_builds WHERE tenant_id=$1 AND hierarchy_type=$2 AND is_active`,
			tenantID,
			hierarchyType,
		).Scan(&builtAt, &buildStatus); err != nil {
			if err == pgx.ErrNoRows {
				return orgComponentHealth{
					Status:       orgHealthStatusDegraded,
					ResponseTime: time.Since(start).String(),
					Error:        "no active closure build",
					Details: map[string]any{
						"backend": string(backend),
					},
				}
			}
			return orgComponentHealth{
				Status:       orgHealthStatusDegraded,
				ResponseTime: time.Since(start).String(),
				Error:        fmt.Sprintf("closure build query failed: %v", err),
				Details: map[string]any{
					"backend": string(backend),
				},
			}
		}
		return buildFreshness("closure", backend, builtAt, buildStatus, start)
	case services.DeepReadBackendSnapshot:
		asOfDate := asOf.UTC().Format("2006-01-02")
		var builtAt time.Time
		var buildStatus string
		if err := db.QueryRow(
			timeoutCtx,
			`SELECT built_at, status FROM org_hierarchy_snapshot_builds WHERE tenant_id=$1 AND hierarchy_type=$2 AND as_of_date=$3 AND is_active`,
			tenantID,
			hierarchyType,
			asOfDate,
		).Scan(&builtAt, &buildStatus); err != nil {
			if err == pgx.ErrNoRows {
				return orgComponentHealth{
					Status:       orgHealthStatusDegraded,
					ResponseTime: time.Since(start).String(),
					Error:        "no active snapshot build",
					Details: map[string]any{
						"backend":    string(backend),
						"as_of_date": asOfDate,
					},
				}
			}
			return orgComponentHealth{
				Status:       orgHealthStatusDegraded,
				ResponseTime: time.Since(start).String(),
				Error:        fmt.Sprintf("snapshot build query failed: %v", err),
				Details: map[string]any{
					"backend":    string(backend),
					"as_of_date": asOfDate,
				},
			}
		}
		health := buildFreshness("snapshot", backend, builtAt, buildStatus, start)
		if health.Details == nil {
			health.Details = map[string]any{}
		}
		health.Details["as_of_date"] = asOfDate
		return health
	default:
		return orgComponentHealth{
			Status:       orgHealthStatusDegraded,
			ResponseTime: time.Since(start).String(),
			Error:        fmt.Sprintf("unknown deep read backend %q", backend),
		}
	}
}

func buildFreshness(kind string, backend services.DeepReadBackend, builtAt time.Time, buildStatus string, started time.Time) orgComponentHealth {
	status := orgHealthStatusHealthy
	details := map[string]any{
		"backend": string(backend),
	}
	details["build_status"] = buildStatus

	age := time.Since(builtAt)
	details["active_build_age"] = age.Truncate(time.Second).String()
	if age > orgDeepReadBuildAgeDegraded {
		status = orgHealthStatusDegraded
	}
	if buildStatus != "ready" {
		status = orgHealthStatusDegraded
	}

	return orgComponentHealth{
		Status:       status,
		ResponseTime: time.Since(started).String(),
		Details:      details,
	}
}
