package controllers

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func (c *OrgAPIController) GetStaffingSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionReportsAuthzObject, "read") {
		return
	}

	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	groupBy := services.StaffingGroupBy(strings.TrimSpace(r.URL.Query().Get("group_by")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	includeSystem := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_system")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include_system is invalid")
			return
		}
		includeSystem = v
	}

	res, err := c.org.GetStaffingSummary(r.Context(), tenantID, services.StaffingSummaryInput{
		OrgNodeID:         orgNodeIDPtr,
		EffectiveDate:     asOf,
		Scope:             scope,
		GroupBy:           groupBy,
		LifecycleStatuses: statuses,
		IncludeSystem:     includeSystem,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type totals struct {
		PositionsTotal int     `json:"positions_total"`
		CapacityFTE    float64 `json:"capacity_fte"`
		OccupiedFTE    float64 `json:"occupied_fte"`
		AvailableFTE   float64 `json:"available_fte"`
		FillRate       float64 `json:"fill_rate"`
	}
	type breakdownItem struct {
		Key            string  `json:"key"`
		PositionsTotal int     `json:"positions_total"`
		CapacityFTE    float64 `json:"capacity_fte"`
		OccupiedFTE    float64 `json:"occupied_fte"`
		AvailableFTE   float64 `json:"available_fte"`
		FillRate       float64 `json:"fill_rate"`
	}
	type source struct {
		DeepReadBackend string  `json:"deep_read_backend"`
		SnapshotBuildID *string `json:"snapshot_build_id,omitempty"`
	}
	type response struct {
		TenantID      string          `json:"tenant_id"`
		OrgNodeID     string          `json:"org_node_id"`
		EffectiveDate string          `json:"effective_date"`
		Scope         string          `json:"scope"`
		Totals        totals          `json:"totals"`
		Breakdown     []breakdownItem `json:"breakdown"`
		Source        source          `json:"source"`
	}

	out := response{
		TenantID:      res.TenantID.String(),
		OrgNodeID:     res.OrgNodeID.String(),
		EffectiveDate: formatValidDate(res.EffectiveDate),
		Scope:         string(res.Scope),
		Totals: totals{
			PositionsTotal: res.Totals.PositionsTotal,
			CapacityFTE:    res.Totals.CapacityFTE,
			OccupiedFTE:    res.Totals.OccupiedFTE,
			AvailableFTE:   res.Totals.AvailableFTE,
			FillRate:       res.Totals.FillRate,
		},
		Breakdown: make([]breakdownItem, 0, len(res.Breakdown)),
		Source: source{
			DeepReadBackend: string(res.Source.DeepReadBackend),
		},
	}
	if res.Source.SnapshotBuildID != nil && *res.Source.SnapshotBuildID != uuid.Nil {
		id := res.Source.SnapshotBuildID.String()
		out.Source.SnapshotBuildID = &id
	}
	for _, b := range res.Breakdown {
		out.Breakdown = append(out.Breakdown, breakdownItem{
			Key:            b.Key,
			PositionsTotal: b.PositionsTotal,
			CapacityFTE:    b.CapacityFTE,
			OccupiedFTE:    b.OccupiedFTE,
			AvailableFTE:   b.AvailableFTE,
			FillRate:       b.FillRate,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

func (c *OrgAPIController) GetStaffingVacancies(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionReportsAuthzObject, "read") {
		return
	}

	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = v
	}

	var cursor *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
			return
		}
		cursor = &id
	}

	includeSystem := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_system")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include_system is invalid")
			return
		}
		includeSystem = v
	}

	res, err := c.org.ListStaffingVacancies(r.Context(), tenantID, services.StaffingVacanciesInput{
		OrgNodeID:         orgNodeIDPtr,
		EffectiveDate:     asOf,
		Scope:             scope,
		LifecycleStatuses: statuses,
		IncludeSystem:     includeSystem,
		Limit:             limit,
		Cursor:            cursor,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type item struct {
		PositionID     string  `json:"position_id"`
		PositionCode   string  `json:"position_code"`
		OrgNodeID      string  `json:"org_node_id"`
		CapacityFTE    float64 `json:"capacity_fte"`
		OccupiedFTE    float64 `json:"occupied_fte"`
		VacancySince   string  `json:"vacancy_since"`
		VacancyAgeDays int     `json:"vacancy_age_days"`
		JobLevelID     *string `json:"job_level_id"`
		PositionType   string  `json:"position_type"`
	}
	type source struct {
		DeepReadBackend string  `json:"deep_read_backend"`
		SnapshotBuildID *string `json:"snapshot_build_id,omitempty"`
	}
	type response struct {
		TenantID      string  `json:"tenant_id"`
		OrgNodeID     string  `json:"org_node_id"`
		EffectiveDate string  `json:"effective_date"`
		Scope         string  `json:"scope"`
		Items         []item  `json:"items"`
		NextCursor    *string `json:"next_cursor"`
		Source        source  `json:"source"`
	}

	out := response{
		TenantID:      res.TenantID.String(),
		OrgNodeID:     res.OrgNodeID.String(),
		EffectiveDate: formatValidDate(res.EffectiveDate),
		Scope:         string(res.Scope),
		Items:         make([]item, 0, len(res.Items)),
		Source: source{
			DeepReadBackend: string(res.Source.DeepReadBackend),
		},
	}
	if res.Source.SnapshotBuildID != nil && *res.Source.SnapshotBuildID != uuid.Nil {
		id := res.Source.SnapshotBuildID.String()
		out.Source.SnapshotBuildID = &id
	}
	if res.NextCursor != nil && *res.NextCursor != uuid.Nil {
		v := res.NextCursor.String()
		out.NextCursor = &v
	}

	for _, it := range res.Items {
		var jobLevelID *string
		if it.JobLevelID != nil && *it.JobLevelID != uuid.Nil {
			v := it.JobLevelID.String()
			jobLevelID = &v
		}
		out.Items = append(out.Items, item{
			PositionID:     it.PositionID.String(),
			PositionCode:   it.PositionCode,
			OrgNodeID:      it.OrgNodeID.String(),
			CapacityFTE:    it.CapacityFTE,
			OccupiedFTE:    it.OccupiedFTE,
			VacancySince:   formatValidDate(it.VacancySince),
			VacancyAgeDays: it.VacancyAgeDays,
			JobLevelID:     jobLevelID,
			PositionType:   it.PositionType,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (c *OrgAPIController) GetStaffingTimeToFill(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionReportsAuthzObject, "read") {
		return
	}

	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}

	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromRaw == "" || toRaw == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "from/to are required")
		return
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "from is invalid")
		return
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "to is invalid")
		return
	}

	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	groupBy := services.StaffingGroupBy(strings.TrimSpace(r.URL.Query().Get("group_by")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	res, err := c.org.GetStaffingTimeToFill(r.Context(), tenantID, services.StaffingTimeToFillInput{
		OrgNodeID:         orgNodeIDPtr,
		From:              from.UTC(),
		To:                to.UTC(),
		Scope:             scope,
		GroupBy:           groupBy,
		LifecycleStatuses: statuses,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type summary struct {
		FilledCount int     `json:"filled_count"`
		AvgDays     float64 `json:"avg_days"`
		P50Days     int     `json:"p50_days"`
		P95Days     int     `json:"p95_days"`
	}
	type breakdownItem struct {
		Key         string  `json:"key"`
		FilledCount int     `json:"filled_count"`
		AvgDays     float64 `json:"avg_days"`
	}
	type source struct {
		DeepReadBackend string  `json:"deep_read_backend"`
		SnapshotBuildID *string `json:"snapshot_build_id,omitempty"`
	}
	type response struct {
		TenantID  string          `json:"tenant_id"`
		OrgNodeID string          `json:"org_node_id"`
		From      string          `json:"from"`
		To        string          `json:"to"`
		Scope     string          `json:"scope"`
		Summary   summary         `json:"summary"`
		Breakdown []breakdownItem `json:"breakdown"`
		Source    source          `json:"source"`
	}

	out := response{
		TenantID:  res.TenantID.String(),
		OrgNodeID: res.OrgNodeID.String(),
		From:      res.From.UTC().Format("2006-01-02"),
		To:        res.To.UTC().Format("2006-01-02"),
		Scope:     string(res.Scope),
		Summary: summary{
			FilledCount: res.Summary.FilledCount,
			AvgDays:     res.Summary.AvgDays,
			P50Days:     res.Summary.P50Days,
			P95Days:     res.Summary.P95Days,
		},
		Breakdown: make([]breakdownItem, 0, len(res.Breakdown)),
		Source: source{
			DeepReadBackend: string(res.Source.DeepReadBackend),
		},
	}
	if res.Source.SnapshotBuildID != nil && *res.Source.SnapshotBuildID != uuid.Nil {
		id := res.Source.SnapshotBuildID.String()
		out.Source.SnapshotBuildID = &id
	}
	for _, b := range res.Breakdown {
		out.Breakdown = append(out.Breakdown, breakdownItem{
			Key:         b.Key,
			FilledCount: b.FilledCount,
			AvgDays:     b.AvgDays,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (c *OrgAPIController) ExportStaffingReport(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionReportsAuthzObject, "read") {
		return
	}

	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	if kind == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "kind is required")
		return
	}

	format := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "format is invalid")
		return
	}

	if format == "json" {
		switch kind {
		case "summary":
			c.GetStaffingSummary(w, r)
		case "vacancies":
			c.GetStaffingVacancies(w, r)
		case "time_to_fill":
			c.GetStaffingTimeToFill(w, r)
		default:
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "kind is invalid")
		}
		return
	}

	switch kind {
	case "summary":
		c.exportStaffingSummary(w, r, requestID, tenantID, format)
	case "vacancies":
		c.exportStaffingVacancies(w, r, requestID, tenantID, format)
	case "time_to_fill":
		c.exportStaffingTimeToFill(w, r, requestID, tenantID, format)
	default:
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "kind is invalid")
	}
}

func (c *OrgAPIController) exportStaffingSummary(w http.ResponseWriter, r *http.Request, requestID string, tenantID uuid.UUID, format string) {
	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}
	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}
	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	groupBy := services.StaffingGroupBy(strings.TrimSpace(r.URL.Query().Get("group_by")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	includeSystem := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_system")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include_system is invalid")
			return
		}
		includeSystem = v
	}

	res, err := c.org.GetStaffingSummary(r.Context(), tenantID, services.StaffingSummaryInput{
		OrgNodeID:         orgNodeIDPtr,
		EffectiveDate:     asOf,
		Scope:             scope,
		GroupBy:           groupBy,
		LifecycleStatuses: statuses,
		IncludeSystem:     includeSystem,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	header := []string{"key", "positions_total", "capacity_fte", "occupied_fte", "available_fte", "fill_rate"}
	rows := make([][]string, 0, 1+len(res.Breakdown))

	rows = append(rows, []string{
		"totals",
		strconv.Itoa(res.Totals.PositionsTotal),
		floatToString(res.Totals.CapacityFTE),
		floatToString(res.Totals.OccupiedFTE),
		floatToString(res.Totals.AvailableFTE),
		floatToString(res.Totals.FillRate),
	})
	for _, b := range res.Breakdown {
		rows = append(rows, []string{
			b.Key,
			strconv.Itoa(b.PositionsTotal),
			floatToString(b.CapacityFTE),
			floatToString(b.OccupiedFTE),
			floatToString(b.AvailableFTE),
			floatToString(b.FillRate),
		})
	}

	writeCSV(w, header, rows)
}

func (c *OrgAPIController) exportStaffingVacancies(w http.ResponseWriter, r *http.Request, requestID string, tenantID uuid.UUID, format string) {
	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}
	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}
	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = v
	}

	var cursor *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
			return
		}
		cursor = &id
	}

	includeSystem := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_system")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include_system is invalid")
			return
		}
		includeSystem = v
	}

	res, err := c.org.ListStaffingVacancies(r.Context(), tenantID, services.StaffingVacanciesInput{
		OrgNodeID:         orgNodeIDPtr,
		EffectiveDate:     asOf,
		Scope:             scope,
		LifecycleStatuses: statuses,
		IncludeSystem:     includeSystem,
		Limit:             limit,
		Cursor:            cursor,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	header := []string{
		"position_id",
		"position_code",
		"org_node_id",
		"capacity_fte",
		"occupied_fte",
		"vacancy_since",
		"vacancy_age_days",
		"job_level_id",
		"position_type",
	}
	rows := make([][]string, 0, len(res.Items))
	for _, it := range res.Items {
		jobLevelID := ""
		if it.JobLevelID != nil && *it.JobLevelID != uuid.Nil {
			jobLevelID = it.JobLevelID.String()
		}
		rows = append(rows, []string{
			it.PositionID.String(),
			it.PositionCode,
			it.OrgNodeID.String(),
			floatToString(it.CapacityFTE),
			floatToString(it.OccupiedFTE),
			formatValidDate(it.VacancySince),
			strconv.Itoa(it.VacancyAgeDays),
			jobLevelID,
			it.PositionType,
		})
	}

	writeCSV(w, header, rows)
}

func (c *OrgAPIController) exportStaffingTimeToFill(w http.ResponseWriter, r *http.Request, requestID string, tenantID uuid.UUID, format string) {
	orgNodeID, hasOrgNodeID, err := parseOptionalUUID(r.URL.Query().Get("org_node_id"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}
	var orgNodeIDPtr *uuid.UUID
	if hasOrgNodeID {
		orgNodeIDPtr = &orgNodeID
	}

	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromRaw == "" || toRaw == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "from/to are required")
		return
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "from is invalid")
		return
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "to is invalid")
		return
	}

	scope := services.StaffingScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	groupBy := services.StaffingGroupBy(strings.TrimSpace(r.URL.Query().Get("group_by")))
	statuses := splitCommaList(r.URL.Query().Get("lifecycle_statuses"))

	res, err := c.org.GetStaffingTimeToFill(r.Context(), tenantID, services.StaffingTimeToFillInput{
		OrgNodeID:         orgNodeIDPtr,
		From:              from.UTC(),
		To:                to.UTC(),
		Scope:             scope,
		GroupBy:           groupBy,
		LifecycleStatuses: statuses,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	header := []string{"key", "filled_count", "avg_days", "p50_days", "p95_days"}
	rows := make([][]string, 0, 1+len(res.Breakdown))

	rows = append(rows, []string{
		"summary",
		strconv.Itoa(res.Summary.FilledCount),
		floatToString(res.Summary.AvgDays),
		strconv.Itoa(res.Summary.P50Days),
		strconv.Itoa(res.Summary.P95Days),
	})
	for _, b := range res.Breakdown {
		rows = append(rows, []string{
			b.Key,
			strconv.Itoa(b.FilledCount),
			floatToString(b.AvgDays),
			"",
			"",
		})
	}

	writeCSV(w, header, rows)
}

func parseOptionalUUID(raw string) (uuid.UUID, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, false, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func splitCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func floatToString(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func writeCSV(w http.ResponseWriter, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	_ = cw.Write(header)
	_ = cw.WriteAll(rows)
	cw.Flush()
}
