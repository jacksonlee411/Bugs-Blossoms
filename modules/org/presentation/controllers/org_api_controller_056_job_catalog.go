package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func (c *OrgAPIController) ListJobFamilyGroups(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	rows, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type createJobFamilyGroupRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive *bool  `json:"is_active"`
}

func (c *OrgAPIController) CreateJobFamilyGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	var req createJobFamilyGroupRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	row, err := c.org.CreateJobFamilyGroup(r.Context(), tenantID, services.JobFamilyGroupCreate{
		Code:     req.Code,
		Name:     req.Name,
		IsActive: isActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

type updateJobFamilyGroupRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func (c *OrgAPIController) UpdateJobFamilyGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req updateJobFamilyGroupRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	row, err := c.org.UpdateJobFamilyGroup(r.Context(), tenantID, id, services.JobFamilyGroupUpdate{
		Name:     req.Name,
		IsActive: req.IsActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (c *OrgAPIController) ListJobFamilies(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	groupIDRaw := strings.TrimSpace(r.URL.Query().Get("job_family_group_id"))
	groupID, err := uuid.Parse(groupIDRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "job_family_group_id is invalid")
		return
	}
	rows, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type createJobFamilyRequest struct {
	JobFamilyGroupID uuid.UUID `json:"job_family_group_id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	IsActive         *bool     `json:"is_active"`
}

func (c *OrgAPIController) CreateJobFamily(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	var req createJobFamilyRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	row, err := c.org.CreateJobFamily(r.Context(), tenantID, services.JobFamilyCreate{
		JobFamilyGroupID: req.JobFamilyGroupID,
		Code:             req.Code,
		Name:             req.Name,
		IsActive:         isActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

type updateJobFamilyRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func (c *OrgAPIController) UpdateJobFamily(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req updateJobFamilyRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	row, err := c.org.UpdateJobFamily(r.Context(), tenantID, id, services.JobFamilyUpdate{
		Name:     req.Name,
		IsActive: req.IsActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (c *OrgAPIController) ListJobRoles(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	familyIDRaw := strings.TrimSpace(r.URL.Query().Get("job_family_id"))
	familyID, err := uuid.Parse(familyIDRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "job_family_id is invalid")
		return
	}
	rows, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type createJobRoleRequest struct {
	JobFamilyID uuid.UUID `json:"job_family_id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	IsActive    *bool     `json:"is_active"`
}

func (c *OrgAPIController) CreateJobRole(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	var req createJobRoleRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	row, err := c.org.CreateJobRole(r.Context(), tenantID, services.JobRoleCreate{
		JobFamilyID: req.JobFamilyID,
		Code:        req.Code,
		Name:        req.Name,
		IsActive:    isActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

type updateJobRoleRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func (c *OrgAPIController) UpdateJobRole(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req updateJobRoleRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	row, err := c.org.UpdateJobRole(r.Context(), tenantID, id, services.JobRoleUpdate{
		Name:     req.Name,
		IsActive: req.IsActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (c *OrgAPIController) ListJobLevels(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	roleIDRaw := strings.TrimSpace(r.URL.Query().Get("job_role_id"))
	roleID, err := uuid.Parse(roleIDRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "job_role_id is invalid")
		return
	}
	rows, err := c.org.ListJobLevels(r.Context(), tenantID, roleID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type createJobLevelRequest struct {
	JobRoleID    uuid.UUID `json:"job_role_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	DisplayOrder int       `json:"display_order"`
	IsActive     *bool     `json:"is_active"`
}

func (c *OrgAPIController) CreateJobLevel(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	var req createJobLevelRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	row, err := c.org.CreateJobLevel(r.Context(), tenantID, services.JobLevelCreate{
		JobRoleID:    req.JobRoleID,
		Code:         req.Code,
		Name:         req.Name,
		DisplayOrder: req.DisplayOrder,
		IsActive:     isActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

type updateJobLevelRequest struct {
	Name         *string `json:"name"`
	DisplayOrder *int    `json:"display_order"`
	IsActive     *bool   `json:"is_active"`
}

func (c *OrgAPIController) UpdateJobLevel(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req updateJobLevelRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	row, err := c.org.UpdateJobLevel(r.Context(), tenantID, id, services.JobLevelUpdate{
		Name:         req.Name,
		DisplayOrder: req.DisplayOrder,
		IsActive:     req.IsActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (c *OrgAPIController) ListJobProfiles(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobProfilesAuthzObject, "read") {
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("job_role_id"))
	var roleID *uuid.UUID
	if raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "job_role_id is invalid")
			return
		}
		roleID = &id
	}
	rows, err := c.org.ListJobProfiles(r.Context(), tenantID, roleID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

type createJobProfileRequest struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	JobRoleID   uuid.UUID `json:"job_role_id"`
	IsActive    *bool     `json:"is_active"`
}

func (c *OrgAPIController) CreateJobProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobProfilesAuthzObject, "admin") {
		return
	}

	var req createJobProfileRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	row, err := c.org.CreateJobProfile(r.Context(), tenantID, services.JobProfileCreate{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		JobRoleID:   req.JobRoleID,
		IsActive:    isActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

type updateJobProfileRequest struct {
	Name        *string        `json:"name"`
	Description optionalString `json:"description"`
	IsActive    *bool          `json:"is_active"`
}

func (c *OrgAPIController) UpdateJobProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobProfilesAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req updateJobProfileRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}

	row, err := c.org.UpdateJobProfile(r.Context(), tenantID, id, services.JobProfileUpdate{
		Name:        req.Name,
		Description: fieldIfSetString(req.Description),
		IsActive:    req.IsActive,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

type setJobProfileAllowedLevelsRequest struct {
	JobLevelIDs []uuid.UUID `json:"job_level_ids"`
}

func (c *OrgAPIController) SetJobProfileAllowedLevels(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgJobProfilesAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req setJobProfileAllowedLevelsRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	if err := c.org.SetJobProfileAllowedLevels(r.Context(), tenantID, id, services.JobProfileAllowedLevelsSet{
		JobLevelIDs: req.JobLevelIDs,
	}); err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_profile_id": id.String()})
}

func (c *OrgAPIController) GetPositionRestrictions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionRestrictionsAuthzObject, "read") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	asOfRaw := strings.TrimSpace(r.URL.Query().Get("effective_date"))
	var asOf *time.Time
	if asOfRaw != "" {
		t, err := parseEffectiveDate(asOfRaw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
			return
		}
		asOf = &t
	}
	res, effectiveDate, err := c.org.GetPositionRestrictions(r.Context(), tenantID, positionID, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id":    tenantID.String(),
		"as_of":        formatValidDate(effectiveDate),
		"restrictions": res,
	})
}

type setPositionRestrictionsRequest struct {
	EffectiveDate        string          `json:"effective_date"`
	PositionRestrictions json.RawMessage `json:"position_restrictions"`
	ReasonCode           string          `json:"reason_code"`
	ReasonNote           *string         `json:"reason_note"`
}

func (c *OrgAPIController) SetPositionRestrictions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionRestrictionsAuthzObject, "admin") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req setPositionRestrictionsRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}
	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.SetPositionRestrictions(r.Context(), tenantID, requestID, initiatorID, services.SetPositionRestrictionsInput{
		PositionID:           positionID,
		EffectiveDate:        effectiveDate,
		PositionRestrictions: req.PositionRestrictions,
		ReasonCode:           req.ReasonCode,
		ReasonNote:           req.ReasonNote,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"position_id": res.PositionID.String(),
		"slice_id":    res.SliceID.String(),
		"effective_window": map[string]any{
			"effective_date": formatValidDate(res.EffectiveDate),
			"end_date":       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}
