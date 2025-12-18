package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	coredtos "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	coreservices "github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type OrgAPIController struct {
	app       application.Application
	org       *services.OrgService
	users     *coreservices.UserService
	apiPrefix string
}

func NewOrgAPIController(app application.Application) application.Controller {
	return &OrgAPIController{
		app:       app,
		org:       app.Service(services.OrgService{}).(*services.OrgService),
		users:     app.Service(coreservices.UserService{}).(*coreservices.UserService),
		apiPrefix: "/org/api",
	}
}

func (c *OrgAPIController) Key() string {
	return c.apiPrefix
}

func (c *OrgAPIController) Register(r *mux.Router) {
	api := r.PathPrefix(c.apiPrefix).Subrouter()
	api.Use(
		middleware.Authorize(),
		middleware.ProvideUser(),
	)

	api.HandleFunc("/hierarchies", c.GetHierarchies).Methods(http.MethodGet)

	api.HandleFunc("/nodes", c.CreateNode).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}", c.UpdateNode).Methods(http.MethodPatch)
	api.HandleFunc("/nodes/{id}:move", c.MoveNode).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:correct", c.CorrectNode).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:rescind", c.RescindNode).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:shift-boundary", c.ShiftBoundaryNode).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:correct-move", c.CorrectMoveNode).Methods(http.MethodPost)

	api.HandleFunc("/assignments", c.GetAssignments).Methods(http.MethodGet)
	api.HandleFunc("/assignments", c.CreateAssignment).Methods(http.MethodPost)
	api.HandleFunc("/assignments/{id}", c.UpdateAssignment).Methods(http.MethodPatch)
	api.HandleFunc("/assignments/{id}:correct", c.CorrectAssignment).Methods(http.MethodPost)
	api.HandleFunc("/assignments/{id}:rescind", c.RescindAssignment).Methods(http.MethodPost)

	api.HandleFunc("/snapshot", c.GetSnapshot).Methods(http.MethodGet)
	api.HandleFunc("/batch", c.Batch).Methods(http.MethodPost)
}

type effectiveWindowResponse struct {
	EffectiveDate string `json:"effective_date"`
	EndDate       string `json:"end_date"`
}

func (c *OrgAPIController) GetHierarchies(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	hType := r.URL.Query().Get("type")
	if strings.TrimSpace(hType) == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "type is required")
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	nodes, effectiveDate, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, hType, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type hierarchiesResponse struct {
		TenantID      string                   `json:"tenant_id"`
		HierarchyType string                   `json:"hierarchy_type"`
		EffectiveDate string                   `json:"effective_date"`
		Nodes         []services.HierarchyNode `json:"nodes"`
	}
	writeJSON(w, http.StatusOK, hierarchiesResponse{
		TenantID:      tenantID.String(),
		HierarchyType: hType,
		EffectiveDate: effectiveDate.UTC().Format(time.RFC3339),
		Nodes:         nodes,
	})
}

type createNodeRequest struct {
	Code          string            `json:"code"`
	Name          string            `json:"name"`
	ParentID      *uuid.UUID        `json:"parent_id"`
	EffectiveDate string            `json:"effective_date"`
	I18nNames     map[string]string `json:"i18n_names"`
	Status        string            `json:"status"`
	DisplayOrder  int               `json:"display_order"`
	LegalEntityID *uuid.UUID        `json:"legal_entity_id"`
	CompanyCode   *string           `json:"company_code"`
	LocationID    *uuid.UUID        `json:"location_id"`
	ManagerUserID *int64            `json:"manager_user_id"`
	ManagerEmail  *string           `json:"manager_email"`
}

func (c *OrgAPIController) CreateNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
		return
	}

	var req createNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}

	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}

	managerUserID := req.ManagerUserID
	if managerUserID == nil && req.ManagerEmail != nil && strings.TrimSpace(*req.ManagerEmail) != "" {
		u, err := c.users.GetByEmail(r.Context(), strings.TrimSpace(*req.ManagerEmail))
		if err != nil || u == nil {
			writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_MANAGER_NOT_FOUND", "manager not found")
			return
		}
		id := int64(u.ID())
		managerUserID = &id
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.CreateNode(r.Context(), tenantID, requestID, initiatorID, services.CreateNodeInput{
		Code:          req.Code,
		Name:          req.Name,
		ParentID:      req.ParentID,
		EffectiveDate: effectiveDate,
		I18nNames:     req.I18nNames,
		Status:        req.Status,
		DisplayOrder:  req.DisplayOrder,
		LegalEntityID: req.LegalEntityID,
		CompanyCode:   req.CompanyCode,
		LocationID:    req.LocationID,
		ManagerUserID: managerUserID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type createNodeResponse struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusCreated, createNodeResponse{
		ID: res.NodeID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type optionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *optionalUUID) UnmarshalJSON(data []byte) error {
	o.Set = true
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := uuid.Parse(s)
	if err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

type optionalInt64 struct {
	Set   bool
	Value *int64
}

func (o *optionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		o.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type updateNodeRequest struct {
	EffectiveDate string            `json:"effective_date"`
	Code          *string           `json:"code"`
	EndDate       *string           `json:"end_date"`
	Name          *string           `json:"name"`
	I18nNames     map[string]string `json:"i18n_names"`
	Status        *string           `json:"status"`
	DisplayOrder  *int              `json:"display_order"`
	LegalEntityID optionalUUID      `json:"legal_entity_id"`
	CompanyCode   optionalString    `json:"company_code"`
	LocationID    optionalUUID      `json:"location_id"`
	ManagerUserID optionalInt64     `json:"manager_user_id"`
}

func (c *OrgAPIController) UpdateNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req updateNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	if req.Code != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "code cannot be updated")
		return
	}
	if req.EndDate != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "end_date is not allowed")
		return
	}

	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}

	var legalEntityID **uuid.UUID
	if req.LegalEntityID.Set {
		legalEntityID = &req.LegalEntityID.Value
	}
	var companyCode **string
	if req.CompanyCode.Set {
		companyCode = &req.CompanyCode.Value
	}
	var locationID **uuid.UUID
	if req.LocationID.Set {
		locationID = &req.LocationID.Value
	}
	var managerUserID **int64
	if req.ManagerUserID.Set {
		managerUserID = &req.ManagerUserID.Value
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.UpdateNode(r.Context(), tenantID, requestID, initiatorID, services.UpdateNodeInput{
		NodeID:        nodeID,
		EffectiveDate: effectiveDate,
		Name:          req.Name,
		I18nNames:     req.I18nNames,
		Status:        req.Status,
		DisplayOrder:  req.DisplayOrder,
		LegalEntityID: legalEntityID,
		CompanyCode:   companyCode,
		LocationID:    locationID,
		ManagerUserID: managerUserID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type updateNodeResponse struct {
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, updateNodeResponse{
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type moveNodeRequest struct {
	EffectiveDate string    `json:"effective_date"`
	NewParentID   uuid.UUID `json:"new_parent_id"`
}

func (c *OrgAPIController) MoveNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgEdgesAuthzObject, "write") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req moveNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}
	if req.NewParentID == uuid.Nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "new_parent_id is required")
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.MoveNode(r.Context(), tenantID, requestID, initiatorID, services.MoveNodeInput{
		NodeID:        nodeID,
		NewParentID:   req.NewParentID,
		EffectiveDate: effectiveDate,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type moveNodeResponse struct {
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, moveNodeResponse{
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type correctNodeRequest struct {
	EffectiveDate string            `json:"effective_date"`
	EndDate       *string           `json:"end_date"`
	Name          *string           `json:"name"`
	I18nNames     map[string]string `json:"i18n_names"`
	Status        *string           `json:"status"`
	DisplayOrder  *int              `json:"display_order"`
	LegalEntityID optionalUUID      `json:"legal_entity_id"`
	CompanyCode   optionalString    `json:"company_code"`
	LocationID    optionalUUID      `json:"location_id"`
	ManagerUserID optionalInt64     `json:"manager_user_id"`
}

func (c *OrgAPIController) CorrectNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgNodesAuthzObject, "admin") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req correctNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	if req.EndDate != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "end_date is not allowed")
		return
	}

	asOf, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}

	var legalEntityID **uuid.UUID
	if req.LegalEntityID.Set {
		legalEntityID = &req.LegalEntityID.Value
	}
	var companyCode **string
	if req.CompanyCode.Set {
		companyCode = &req.CompanyCode.Value
	}
	var locationID **uuid.UUID
	if req.LocationID.Set {
		locationID = &req.LocationID.Value
	}
	var managerUserID **int64
	if req.ManagerUserID.Set {
		managerUserID = &req.ManagerUserID.Value
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.CorrectNode(r.Context(), tenantID, requestID, initiatorID, services.CorrectNodeInput{
		NodeID:        nodeID,
		AsOf:          asOf,
		Name:          req.Name,
		I18nNames:     req.I18nNames,
		Status:        req.Status,
		DisplayOrder:  req.DisplayOrder,
		LegalEntityID: legalEntityID,
		CompanyCode:   companyCode,
		LocationID:    locationID,
		ManagerUserID: managerUserID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type correctNodeResponse struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, correctNodeResponse{
		ID: res.NodeID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type rescindNodeRequest struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
}

func (c *OrgAPIController) RescindNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgNodesAuthzObject, "admin") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req rescindNodeRequest
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
	res, err := c.org.RescindNode(r.Context(), tenantID, requestID, initiatorID, services.RescindNodeInput{
		NodeID:        nodeID,
		EffectiveDate: effectiveDate,
		Reason:        req.Reason,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type rescindNodeResponse struct {
		ID              string                  `json:"id"`
		Status          string                  `json:"status"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, rescindNodeResponse{
		ID:     res.NodeID.String(),
		Status: res.Status,
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type shiftBoundaryNodeRequest struct {
	TargetEffectiveDate string `json:"target_effective_date"`
	NewEffectiveDate    string `json:"new_effective_date"`
}

func (c *OrgAPIController) ShiftBoundaryNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgNodesAuthzObject, "admin") {
		return
	}
	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req shiftBoundaryNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	target, err := parseRequiredEffectiveDate(req.TargetEffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "target_effective_date is required")
		return
	}
	newStart, err := parseRequiredEffectiveDate(req.NewEffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "new_effective_date is required")
		return
	}
	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.ShiftBoundaryNode(r.Context(), tenantID, requestID, initiatorID, services.ShiftBoundaryNodeInput{
		NodeID:              nodeID,
		TargetEffectiveDate: target,
		NewEffectiveDate:    newStart,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	type shiftBoundaryResponse struct {
		ID      string `json:"id"`
		Shifted struct {
			TargetEffectiveDate string `json:"target_effective_date"`
			NewEffectiveDate    string `json:"new_effective_date"`
		} `json:"shifted"`
	}
	var resp shiftBoundaryResponse
	resp.ID = res.NodeID.String()
	resp.Shifted.TargetEffectiveDate = res.TargetStart.UTC().Format(time.RFC3339)
	resp.Shifted.NewEffectiveDate = res.NewStart.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, resp)
}

type correctMoveNodeRequest struct {
	EffectiveDate string    `json:"effective_date"`
	NewParentID   uuid.UUID `json:"new_parent_id"`
}

func (c *OrgAPIController) CorrectMoveNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgEdgesAuthzObject, "admin") {
		return
	}
	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req correctMoveNodeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}
	if req.NewParentID == uuid.Nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "new_parent_id is required")
		return
	}
	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.CorrectMoveNode(r.Context(), tenantID, requestID, initiatorID, services.CorrectMoveNodeInput{
		NodeID:        nodeID,
		EffectiveDate: effectiveDate,
		NewParentID:   req.NewParentID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	type correctMoveResponse struct {
		ID            string `json:"id"`
		EffectiveDate string `json:"effective_date"`
	}
	writeJSON(w, http.StatusOK, correctMoveResponse{
		ID:            res.NodeID.String(),
		EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
	})
}

func (c *OrgAPIController) GetAssignments(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "read") {
		return
	}

	subject := r.URL.Query().Get("subject")
	if strings.TrimSpace(subject) == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "subject is required")
		return
	}

	asOfRaw := r.URL.Query().Get("effective_date")
	var asOf *time.Time
	if strings.TrimSpace(asOfRaw) != "" {
		t, err := parseEffectiveDate(asOfRaw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
			return
		}
		asOf = &t
	}

	subjectID, rows, effectiveDate, err := c.org.GetAssignments(r.Context(), tenantID, subject, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type getAssignmentsResponse struct {
		TenantID      string                       `json:"tenant_id"`
		Subject       string                       `json:"subject"`
		SubjectID     string                       `json:"subject_id"`
		EffectiveDate *string                      `json:"effective_date,omitempty"`
		Assignments   []services.AssignmentViewRow `json:"assignments"`
	}
	var effectiveDateOut *string
	if !effectiveDate.IsZero() {
		v := effectiveDate.UTC().Format(time.RFC3339)
		effectiveDateOut = &v
	}
	writeJSON(w, http.StatusOK, getAssignmentsResponse{
		TenantID:      tenantID.String(),
		Subject:       subject,
		SubjectID:     subjectID.String(),
		EffectiveDate: effectiveDateOut,
		Assignments:   rows,
	})
}

type createAssignmentRequest struct {
	Pernr          string     `json:"pernr"`
	EffectiveDate  string     `json:"effective_date"`
	AssignmentType string     `json:"assignment_type"`
	PositionID     *uuid.UUID `json:"position_id"`
	OrgNodeID      *uuid.UUID `json:"org_node_id"`
	SubjectID      *uuid.UUID `json:"subject_id"`
}

func (c *OrgAPIController) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	var req createAssignmentRequest
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
	res, err := c.org.CreateAssignment(r.Context(), tenantID, requestID, initiatorID, services.CreateAssignmentInput{
		Pernr:          req.Pernr,
		EffectiveDate:  effectiveDate,
		AssignmentType: req.AssignmentType,
		PositionID:     req.PositionID,
		OrgNodeID:      req.OrgNodeID,
		SubjectID:      req.SubjectID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type createAssignmentResponse struct {
		AssignmentID    string                  `json:"assignment_id"`
		PositionID      string                  `json:"position_id"`
		SubjectID       string                  `json:"subject_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusCreated, createAssignmentResponse{
		AssignmentID: res.AssignmentID.String(),
		PositionID:   res.PositionID.String(),
		SubjectID:    res.SubjectID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type updateAssignmentRequest struct {
	EffectiveDate string       `json:"effective_date"`
	EndDate       *string      `json:"end_date"`
	PositionID    optionalUUID `json:"position_id"`
	OrgNodeID     optionalUUID `json:"org_node_id"`
}

func (c *OrgAPIController) UpdateAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req updateAssignmentRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	if req.EndDate != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "end_date is not allowed")
		return
	}

	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}

	var positionID *uuid.UUID
	if req.PositionID.Set {
		positionID = req.PositionID.Value
	}
	var orgNodeID *uuid.UUID
	if req.OrgNodeID.Set {
		orgNodeID = req.OrgNodeID.Value
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.UpdateAssignment(r.Context(), tenantID, requestID, initiatorID, services.UpdateAssignmentInput{
		AssignmentID:  assignmentID,
		EffectiveDate: effectiveDate,
		PositionID:    positionID,
		OrgNodeID:     orgNodeID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type updateAssignmentResponse struct {
		AssignmentID    string                  `json:"assignment_id"`
		PositionID      string                  `json:"position_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, updateAssignmentResponse{
		AssignmentID: res.AssignmentID.String(),
		PositionID:   res.PositionID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type correctAssignmentRequest struct {
	Pernr      *string    `json:"pernr"`
	PositionID *uuid.UUID `json:"position_id"`
	SubjectID  *uuid.UUID `json:"subject_id"`
}

func (c *OrgAPIController) CorrectAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "admin") {
		return
	}
	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req correctAssignmentRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.CorrectAssignment(r.Context(), tenantID, requestID, initiatorID, services.CorrectAssignmentInput{
		AssignmentID: assignmentID,
		Pernr:        req.Pernr,
		PositionID:   req.PositionID,
		SubjectID:    req.SubjectID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type correctAssignmentResponse struct {
		AssignmentID    string                  `json:"assignment_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, correctAssignmentResponse{
		AssignmentID: res.AssignmentID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

type rescindAssignmentRequest struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
}

func (c *OrgAPIController) RescindAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "admin") {
		return
	}
	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req rescindAssignmentRequest
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
	res, err := c.org.RescindAssignment(r.Context(), tenantID, requestID, initiatorID, services.RescindAssignmentInput{
		AssignmentID:  assignmentID,
		EffectiveDate: effectiveDate,
		Reason:        req.Reason,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	type rescindAssignmentResponse struct {
		AssignmentID    string                  `json:"assignment_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, rescindAssignmentResponse{
		AssignmentID: res.AssignmentID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: res.EffectiveDate.UTC().Format(time.RFC3339),
			EndDate:       res.EndDate.UTC().Format(time.RFC3339),
		},
	})
}

func (c *OrgAPIController) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgSnapshotAuthzObject, "admin") {
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	var includes []string
	if raw := strings.TrimSpace(r.URL.Query().Get("include")); raw != "" {
		includes = strings.Split(raw, ",")
	}

	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = n
	}

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))

	res, err := c.org.GetSnapshot(r.Context(), tenantID, asOf, includes, limit, cursor)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type batchRequest struct {
	DryRun        bool           `json:"dry_run"`
	EffectiveDate string         `json:"effective_date"`
	Commands      []batchCommand `json:"commands"`
}

type batchCommand struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type batchCommandResult struct {
	Index  int            `json:"index"`
	Type   string         `json:"type"`
	Ok     bool           `json:"ok"`
	Result map[string]any `json:"result,omitempty"`
}

func (c *OrgAPIController) Batch(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgBatchAuthzObject, "admin") {
		return
	}

	var req batchRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_BATCH_INVALID_BODY", "invalid json body")
		return
	}

	if len(req.Commands) < 1 || len(req.Commands) > 100 {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_BATCH_TOO_LARGE", "commands size is invalid")
		return
	}

	moves := 0
	for _, cmd := range req.Commands {
		switch strings.TrimSpace(cmd.Type) {
		case "node.move", "node.correct_move":
			moves++
		}
	}
	if moves > 10 {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_BATCH_TOO_MANY_MOVES", "too many move commands")
		return
	}

	globalEffective := strings.TrimSpace(req.EffectiveDate)
	if globalEffective == "" {
		globalEffective = time.Now().UTC().Format(time.RFC3339)
	} else {
		if _, err := parseEffectiveDate(globalEffective); err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_BATCH_INVALID_BODY", "effective_date is invalid")
			return
		}
	}

	pool, err := composables.UsePool(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, requestID, "ORG_INTERNAL", err.Error())
		return
	}
	tx, err := pool.Begin(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, requestID, "ORG_INTERNAL", err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	txCtx := composables.WithTx(r.Context(), tx)
	txCtx = composables.WithTenantID(txCtx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, requestID, "ORG_INTERNAL", err.Error())
		return
	}

	txCtx = services.WithSkipCacheInvalidation(txCtx)
	if req.DryRun {
		txCtx = services.WithSkipOutboxEnqueue(txCtx)
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	results := make([]batchCommandResult, 0, len(req.Commands))
	eventsEnqueued := 0

	for i, cmd := range req.Commands {
		cmdType := strings.TrimSpace(cmd.Type)
		if cmdType == "" {
			writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "type is required")
			return
		}
		payload := cmd.Payload
		if len(payload) == 0 {
			writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is required")
			return
		}
		payload, err = injectEffectiveDate(payload, globalEffective)
		if err != nil {
			writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
			return
		}

		switch cmdType {
		case "node.create":
			var body createNodeRequest
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CreateNode(txCtx, tenantID, requestID, initiatorID, services.CreateNodeInput{
				Code:          body.Code,
				Name:          body.Name,
				ParentID:      body.ParentID,
				EffectiveDate: effectiveDate,
				I18nNames:     body.I18nNames,
				Status:        body.Status,
				DisplayOrder:  body.DisplayOrder,
				LegalEntityID: body.LegalEntityID,
				CompanyCode:   body.CompanyCode,
				LocationID:    body.LocationID,
				ManagerUserID: body.ManagerUserID,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.update":
			var body struct {
				ID uuid.UUID `json:"id"`
				updateNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.UpdateNode(txCtx, tenantID, requestID, initiatorID, services.UpdateNodeInput{
				NodeID:        body.ID,
				EffectiveDate: effectiveDate,
				Name:          body.Name,
				I18nNames:     body.I18nNames,
				Status:        body.Status,
				DisplayOrder:  body.DisplayOrder,
				LegalEntityID: fieldIfSetUUID(body.LegalEntityID),
				CompanyCode:   fieldIfSetString(body.CompanyCode),
				LocationID:    fieldIfSetUUID(body.LocationID),
				ManagerUserID: fieldIfSetInt64(body.ManagerUserID),
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.move":
			var body struct {
				ID uuid.UUID `json:"id"`
				moveNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.MoveNode(txCtx, tenantID, requestID, initiatorID, services.MoveNodeInput{
				NodeID:        body.ID,
				NewParentID:   body.NewParentID,
				EffectiveDate: effectiveDate,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.EdgeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.correct":
			var body struct {
				ID uuid.UUID `json:"id"`
				correctNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			asOf, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CorrectNode(txCtx, tenantID, requestID, initiatorID, services.CorrectNodeInput{
				NodeID:        body.ID,
				AsOf:          asOf,
				Name:          body.Name,
				I18nNames:     body.I18nNames,
				Status:        body.Status,
				DisplayOrder:  body.DisplayOrder,
				LegalEntityID: fieldIfSetUUID(body.LegalEntityID),
				CompanyCode:   fieldIfSetString(body.CompanyCode),
				LocationID:    fieldIfSetUUID(body.LocationID),
				ManagerUserID: fieldIfSetInt64(body.ManagerUserID),
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.rescind":
			var body struct {
				ID uuid.UUID `json:"id"`
				rescindNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.RescindNode(txCtx, tenantID, requestID, initiatorID, services.RescindNodeInput{
				NodeID:        body.ID,
				EffectiveDate: effectiveDate,
				Reason:        body.Reason,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.shift_boundary":
			var body struct {
				ID uuid.UUID `json:"id"`
				shiftBoundaryNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			target, err := parseRequiredEffectiveDate(body.TargetEffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "target_effective_date is required")
				return
			}
			newStart, err := parseRequiredEffectiveDate(body.NewEffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "new_effective_date is required")
				return
			}
			res, err := c.org.ShiftBoundaryNode(txCtx, tenantID, requestID, initiatorID, services.ShiftBoundaryNodeInput{
				NodeID:              body.ID,
				TargetEffectiveDate: target,
				NewEffectiveDate:    newStart,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "node.correct_move":
			var body struct {
				ID uuid.UUID `json:"id"`
				correctMoveNodeRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CorrectMoveNode(txCtx, tenantID, requestID, initiatorID, services.CorrectMoveNodeInput{
				NodeID:        body.ID,
				EffectiveDate: effectiveDate,
				NewParentID:   body.NewParentID,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.NodeID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "assignment.create":
			var body createAssignmentRequest
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CreateAssignment(txCtx, tenantID, requestID, initiatorID, services.CreateAssignmentInput{
				Pernr:          body.Pernr,
				EffectiveDate:  effectiveDate,
				AssignmentType: body.AssignmentType,
				PositionID:     body.PositionID,
				OrgNodeID:      body.OrgNodeID,
				SubjectID:      body.SubjectID,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.AssignmentID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "assignment.update":
			var body struct {
				ID uuid.UUID `json:"id"`
				updateAssignmentRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			var positionID *uuid.UUID
			if body.PositionID.Set && body.PositionID.Value != nil {
				positionID = body.PositionID.Value
			}
			var orgNodeID *uuid.UUID
			if body.OrgNodeID.Set && body.OrgNodeID.Value != nil {
				orgNodeID = body.OrgNodeID.Value
			}
			res, err := c.org.UpdateAssignment(txCtx, tenantID, requestID, initiatorID, services.UpdateAssignmentInput{
				AssignmentID:  body.ID,
				EffectiveDate: effectiveDate,
				PositionID:    positionID,
				OrgNodeID:     orgNodeID,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.AssignmentID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "assignment.correct":
			var body struct {
				ID uuid.UUID `json:"id"`
				correctAssignmentRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			res, err := c.org.CorrectAssignment(txCtx, tenantID, requestID, initiatorID, services.CorrectAssignmentInput{
				AssignmentID: body.ID,
				Pernr:        body.Pernr,
				PositionID:   body.PositionID,
				SubjectID:    body.SubjectID,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.AssignmentID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		case "assignment.rescind":
			var body struct {
				ID uuid.UUID `json:"id"`
				rescindAssignmentRequest
			}
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.RescindAssignment(txCtx, tenantID, requestID, initiatorID, services.RescindAssignmentInput{
				AssignmentID:  body.ID,
				EffectiveDate: effectiveDate,
				Reason:        body.Reason,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.AssignmentID.String()}})
			if !req.DryRun {
				eventsEnqueued += len(res.GeneratedEvents)
			}

		default:
			writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "unknown type")
			return
		}
	}

	if req.DryRun {
		_ = tx.Rollback(r.Context())
		type batchResponse struct {
			DryRun         bool                 `json:"dry_run"`
			Results        []batchCommandResult `json:"results"`
			EventsEnqueued int                  `json:"events_enqueued"`
		}
		writeJSON(w, http.StatusOK, batchResponse{
			DryRun:         true,
			Results:        results,
			EventsEnqueued: 0,
		})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeAPIError(w, http.StatusInternalServerError, requestID, "ORG_INTERNAL", err.Error())
		return
	}
	c.org.InvalidateTenantCache(tenantID)

	type batchResponse struct {
		DryRun         bool                 `json:"dry_run"`
		Results        []batchCommandResult `json:"results"`
		EventsEnqueued int                  `json:"events_enqueued"`
	}
	writeJSON(w, http.StatusOK, batchResponse{
		DryRun:         false,
		Results:        results,
		EventsEnqueued: eventsEnqueued,
	})
}

func injectEffectiveDate(payload json.RawMessage, effectiveDate string) (json.RawMessage, error) {
	if strings.TrimSpace(effectiveDate) == "" {
		return payload, nil
	}
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if _, ok := obj["effective_date"]; !ok {
		obj["effective_date"] = effectiveDate
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func writeBatchCommandError(w http.ResponseWriter, requestID string, commandIndex int, commandType string, status int, code, message string) {
	meta := map[string]string{
		"request_id":    requestID,
		"command_index": strconv.Itoa(commandIndex),
		"command_type":  commandType,
	}
	writeJSON(w, status, coredtos.APIError{
		Code:    code,
		Message: message,
		Meta:    meta,
	})
}

func writeBatchServiceError(w http.ResponseWriter, requestID string, commandIndex int, commandType string, err error) {
	var svcErr *services.ServiceError
	if errors.As(err, &svcErr) {
		meta := map[string]string{
			"request_id":    requestID,
			"command_index": strconv.Itoa(commandIndex),
			"command_type":  commandType,
		}
		writeJSON(w, svcErr.Status, coredtos.APIError{
			Code:    svcErr.Code,
			Message: svcErr.Message,
			Meta:    meta,
		})
		return
	}
	writeBatchCommandError(w, requestID, commandIndex, commandType, http.StatusInternalServerError, "ORG_INTERNAL", err.Error())
}

func fieldIfSetUUID(v optionalUUID) **uuid.UUID {
	if !v.Set {
		return nil
	}
	return &v.Value
}

func fieldIfSetString(v optionalString) **string {
	if !v.Set {
		return nil
	}
	return &v.Value
}

func fieldIfSetInt64(v optionalInt64) **int64 {
	if !v.Set {
		return nil
	}
	return &v.Value
}

func requireSessionAndTenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	requestID := ensureRequestID(r)

	_, sessErr := composables.UseSession(r.Context())
	if sessErr != nil {
		writeAPIError(w, http.StatusUnauthorized, requestID, "ORG_NO_SESSION", "no session")
		return uuid.Nil, requestID, false
	}
	tid, err := composables.UseTenantID(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_NO_TENANT", "no tenant")
		return uuid.Nil, requestID, false
	}
	return tid, requestID, true
}

func requireSessionTenantUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, coreuser.User, string, bool) {
	tenantID, requestID, ok := requireSessionAndTenant(w, r)
	if !ok {
		return uuid.Nil, nil, requestID, false
	}
	if !requireOrgRolloutEnabled(w, requestID, tenantID) {
		return uuid.Nil, nil, requestID, false
	}
	u, err := composables.UseUser(r.Context())
	if err != nil || u == nil {
		writeAPIError(w, http.StatusUnauthorized, requestID, "ORG_NO_SESSION", "no user")
		return uuid.Nil, nil, requestID, false
	}
	return tenantID, u, requestID, true
}

func requireOrgRolloutEnabled(w http.ResponseWriter, requestID string, tenantID uuid.UUID) bool {
	if services.OrgRolloutEnabledForTenant(tenantID) {
		return true
	}
	writeAPIError(w, http.StatusNotFound, requestID, "ORG_ROLLOUT_DISABLED", "org is not enabled for this tenant")
	return false
}

func ensureRequestID(r *http.Request) string {
	conf := configuration.Use()
	v := strings.TrimSpace(r.Header.Get(conf.RequestIDHeader))
	if v != "" {
		return v
	}
	v = uuid.NewString()
	r.Header.Set(conf.RequestIDHeader, v)
	return v
}

func parseEffectiveDate(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func parseRequiredEffectiveDate(v string) (time.Time, error) {
	t, err := parseEffectiveDate(v)
	if err != nil {
		return time.Time{}, err
	}
	if t.IsZero() {
		return time.Time{}, errors.New("effective_date is required")
	}
	return t, nil
}

func decodeJSON(body io.ReadCloser, out any) error {
	defer func() { _ = body.Close() }()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func writeServiceError(w http.ResponseWriter, requestID string, err error) {
	var svcErr *services.ServiceError
	if errors.As(err, &svcErr) {
		writeAPIError(w, svcErr.Status, requestID, svcErr.Code, svcErr.Message)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, requestID, "ORG_INTERNAL", err.Error())
}

func writeAPIError(w http.ResponseWriter, status int, requestID, code, message string) {
	meta := map[string]string{}
	if requestID != "" {
		meta["request_id"] = requestID
	}
	writeJSON(w, status, coredtos.APIError{
		Code:    code,
		Message: message,
		Meta:    meta,
	})
}

func writeJSON[T any](w http.ResponseWriter, status int, payload T) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
