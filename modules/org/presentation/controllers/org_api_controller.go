package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/iota-uz/iota-sdk/modules/org/domain/changerequest"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type OrgAPIController struct {
	app            application.Application
	org            *services.OrgService
	changeRequests *services.ChangeRequestService
	users          *coreservices.UserService
	apiPrefix      string
}

func NewOrgAPIController(app application.Application) application.Controller {
	return &OrgAPIController{
		app:            app,
		org:            app.Service(services.OrgService{}).(*services.OrgService),
		changeRequests: app.Service(services.ChangeRequestService{}).(*services.ChangeRequestService),
		users:          app.Service(coreservices.UserService{}).(*coreservices.UserService),
		apiPrefix:      "/org/api",
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

	api.HandleFunc("/change-requests", c.CreateChangeRequest).Methods(http.MethodPost)
	api.HandleFunc("/change-requests", c.ListChangeRequests).Methods(http.MethodGet)
	api.HandleFunc("/change-requests/{id}", c.GetChangeRequest).Methods(http.MethodGet)
	api.HandleFunc("/change-requests/{id}", c.UpdateChangeRequest).Methods(http.MethodPatch)
	api.HandleFunc("/change-requests/{id}:submit", c.SubmitChangeRequest).Methods(http.MethodPost)
	api.HandleFunc("/change-requests/{id}:cancel", c.CancelChangeRequest).Methods(http.MethodPost)

	api.HandleFunc("/preflight", c.Preflight).Methods(http.MethodPost)
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

type changeRequestWriteRequest struct {
	Notes   *string         `json:"notes,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

type changeRequestSummaryResponse struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (c *OrgAPIController) CreateChangeRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "write") {
		return
	}

	var req changeRequestWriteRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_CHANGE_REQUEST_INVALID_BODY", "invalid json body")
		return
	}
	if err := validateChangeRequestPayload(req.Payload); err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_CHANGE_REQUEST_INVALID_BODY", err.Error())
		return
	}

	requestID = ensureRequestID(r)
	requesterID := authzutil.NormalizedUserUUID(tenantID, currentUser)

	cr, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) (*changerequest.ChangeRequest, error) {
		return c.changeRequests.SaveDraft(txCtx, services.SaveDraftChangeRequestParams{
			RequestID:   requestID,
			RequesterID: requesterID,
			Payload:     req.Payload,
			Notes:       req.Notes,
		})
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	writeJSON(w, http.StatusCreated, changeRequestSummaryResponse{
		ID:        cr.ID.String(),
		RequestID: cr.RequestID,
		Status:    cr.Status,
		CreatedAt: cr.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: cr.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

type changeRequestListResponse struct {
	Total      int                     `json:"total"`
	Items      []changeRequestListItem `json:"items"`
	NextCursor *string                 `json:"next_cursor"`
}

type changeRequestListItem struct {
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`
	RequesterID string `json:"requester_id"`
	UpdatedAt   string `json:"updated_at"`
}

func (c *OrgAPIController) ListChangeRequests(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "read") {
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	switch status {
	case "", "draft", "submitted", "cancelled":
	default:
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "status is invalid")
		return
	}

	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = n
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	var cursorAt *time.Time
	var cursorID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		at, id, err := parseChangeRequestCursor(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
			return
		}
		cursorAt, cursorID = at, id
	}

	// Fetch one extra row to detect next_cursor.
	rows, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) ([]*changerequest.ChangeRequest, error) {
		txCtx = composables.WithTenantID(txCtx, tenantID)
		return c.changeRequests.List(txCtx, status, limit+1, cursorAt, cursorID)
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	nextCursor := (*string)(nil)
	if len(rows) > limit {
		last := rows[limit-1]
		v := fmt.Sprintf("updated_at:%s:id:%s", last.UpdatedAt.UTC().Format(time.RFC3339), last.ID.String())
		nextCursor = &v
		rows = rows[:limit]
	}

	items := make([]changeRequestListItem, 0, len(rows))
	for _, cr := range rows {
		items = append(items, changeRequestListItem{
			ID:          cr.ID.String(),
			RequestID:   cr.RequestID,
			Status:      cr.Status,
			RequesterID: cr.RequesterID.String(),
			UpdatedAt:   cr.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, changeRequestListResponse{
		Total:      len(items),
		Items:      items,
		NextCursor: nextCursor,
	})
}

type changeRequestDetailResponse struct {
	ID                   string          `json:"id"`
	TenantID             string          `json:"tenant_id"`
	RequestID            string          `json:"request_id"`
	RequesterID          string          `json:"requester_id"`
	Status               string          `json:"status"`
	PayloadSchemaVersion int32           `json:"payload_schema_version"`
	Payload              json.RawMessage `json:"payload"`
	Notes                *string         `json:"notes,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

func (c *OrgAPIController) GetChangeRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "read") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	cr, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) (*changerequest.ChangeRequest, error) {
		txCtx = composables.WithTenantID(txCtx, tenantID)
		return c.changeRequests.Get(txCtx, id)
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	writeJSON(w, http.StatusOK, changeRequestDetailResponse{
		ID:                   cr.ID.String(),
		TenantID:             cr.TenantID.String(),
		RequestID:            cr.RequestID,
		RequesterID:          cr.RequesterID.String(),
		Status:               cr.Status,
		PayloadSchemaVersion: cr.PayloadSchemaVersion,
		Payload:              cr.Payload,
		Notes:                cr.Notes,
		CreatedAt:            cr.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            cr.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (c *OrgAPIController) UpdateChangeRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "write") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req changeRequestWriteRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_CHANGE_REQUEST_INVALID_BODY", "invalid json body")
		return
	}
	if err := validateChangeRequestPayload(req.Payload); err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_CHANGE_REQUEST_INVALID_BODY", err.Error())
		return
	}

	cr, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) (*changerequest.ChangeRequest, error) {
		txCtx = composables.WithTenantID(txCtx, tenantID)
		return c.changeRequests.UpdateDraft(txCtx, services.UpdateDraftChangeRequestParams{
			ID:      id,
			Payload: req.Payload,
			Notes:   req.Notes,
		})
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	writeJSON(w, http.StatusOK, changeRequestSummaryResponse{
		ID:        cr.ID.String(),
		RequestID: cr.RequestID,
		Status:    cr.Status,
		CreatedAt: cr.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: cr.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (c *OrgAPIController) SubmitChangeRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	cr, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) (*changerequest.ChangeRequest, error) {
		txCtx = composables.WithTenantID(txCtx, tenantID)
		return c.changeRequests.Submit(txCtx, id)
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, changeRequestSummaryResponse{
		ID:        cr.ID.String(),
		RequestID: cr.RequestID,
		Status:    cr.Status,
		CreatedAt: cr.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: cr.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (c *OrgAPIController) CancelChangeRequest(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgChangeRequestsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgChangeRequestsAuthzObj, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	cr, err := withOrgTx(r.Context(), tenantID, func(txCtx context.Context) (*changerequest.ChangeRequest, error) {
		txCtx = composables.WithTenantID(txCtx, tenantID)
		return c.changeRequests.Cancel(txCtx, id)
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, changeRequestSummaryResponse{
		ID:        cr.ID.String(),
		RequestID: cr.RequestID,
		Status:    cr.Status,
		CreatedAt: cr.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: cr.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

type preflightRequest struct {
	EffectiveDate string         `json:"effective_date"`
	Commands      []batchCommand `json:"commands"`
}

type preflightResponse struct {
	EffectiveDate string          `json:"effective_date"`
	CommandsCount int             `json:"commands_count"`
	Impact        preflightImpact `json:"impact"`
	Warnings      []string        `json:"warnings"`
}

type preflightImpact struct {
	OrgNodes       preflightCounters `json:"org_nodes"`
	OrgAssignments preflightCounters `json:"org_assignments"`
	Events         map[string]int    `json:"events"`
	Affected       preflightAffected `json:"affected"`
}

type preflightCounters struct {
	Create  int `json:"create"`
	Update  int `json:"update"`
	Move    int `json:"move"`
	Rescind int `json:"rescind"`
}

type preflightAffected struct {
	OrgNodeIDsCount  int      `json:"org_node_ids_count"`
	OrgNodeIDsSample []string `json:"org_node_ids_sample"`
}

func (c *OrgAPIController) Preflight(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgPreflightEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPreflightAuthzObject, "admin") {
		return
	}

	var req preflightRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_PREFLIGHT_INVALID_BODY", "invalid json body")
		return
	}
	if len(req.Commands) < 1 || len(req.Commands) > 100 {
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_PREFLIGHT_TOO_LARGE", "commands size is invalid")
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
		writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_PREFLIGHT_TOO_LARGE", "too many move commands")
		return
	}

	globalEffective := strings.TrimSpace(req.EffectiveDate)
	if globalEffective == "" {
		globalEffective = time.Now().UTC().Format(time.RFC3339)
	} else {
		if _, err := parseEffectiveDate(globalEffective); err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, requestID, "ORG_PREFLIGHT_INVALID_BODY", "effective_date is invalid")
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
	txCtx = services.WithSkipOutboxEnqueue(txCtx)

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)

	// Validation stage: execute commands in a rolled-back transaction.
	for i, cmd := range req.Commands {
		cmdType := strings.TrimSpace(cmd.Type)
		if cmdType == "" {
			writePreflightCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_PREFLIGHT_INVALID_COMMAND", "type is required")
			return
		}
		payload := cmd.Payload
		if len(payload) == 0 {
			writePreflightCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_PREFLIGHT_INVALID_COMMAND", "payload is required")
			return
		}
		payload, err = injectEffectiveDate(payload, globalEffective)
		if err != nil {
			writePreflightCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_PREFLIGHT_INVALID_COMMAND", "payload is invalid")
			return
		}

		if err := c.executePreflightCommand(txCtx, tenantID, requestID, initiatorID, cmdType, payload); err != nil {
			var svcErr *services.ServiceError
			if errors.As(err, &svcErr) {
				writePreflightCommandError(w, requestID, i, cmdType, svcErr.Status, svcErr.Code, svcErr.Message)
				return
			}
			writePreflightCommandError(w, requestID, i, cmdType, http.StatusInternalServerError, "ORG_INTERNAL", err.Error())
			return
		}
	}

	// Always rollback (no writes).
	_ = tx.Rollback(r.Context())

	impact, err := c.analyzePreflightImpact(r.Context(), tenantID, globalEffective, req.Commands)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	asOf, _ := parseEffectiveDate(globalEffective)
	writeJSON(w, http.StatusOK, preflightResponse{
		EffectiveDate: asOf.UTC().Format(time.RFC3339),
		CommandsCount: len(req.Commands),
		Impact:        impact,
		Warnings:      []string{},
	})
}

func (c *OrgAPIController) executePreflightCommand(ctx context.Context, tenantID uuid.UUID, requestID string, initiatorID uuid.UUID, cmdType string, payload json.RawMessage) error {
	switch cmdType {
	case "node.create":
		var body createNodeRequest
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.CreateNode(ctx, tenantID, requestID, initiatorID, services.CreateNodeInput{
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
		return err
	case "node.update":
		var body struct {
			ID uuid.UUID `json:"id"`
			updateNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		var legalEntityID **uuid.UUID
		if body.LegalEntityID.Set {
			legalEntityID = &body.LegalEntityID.Value
		}
		var companyCode **string
		if body.CompanyCode.Set {
			companyCode = &body.CompanyCode.Value
		}
		var locationID **uuid.UUID
		if body.LocationID.Set {
			locationID = &body.LocationID.Value
		}
		var managerUserID **int64
		if body.ManagerUserID.Set {
			managerUserID = &body.ManagerUserID.Value
		}
		_, err = c.org.UpdateNode(ctx, tenantID, requestID, initiatorID, services.UpdateNodeInput{
			NodeID:        body.ID,
			EffectiveDate: effectiveDate,
			Name:          body.Name,
			I18nNames:     body.I18nNames,
			Status:        body.Status,
			DisplayOrder:  body.DisplayOrder,
			LegalEntityID: legalEntityID,
			CompanyCode:   companyCode,
			LocationID:    locationID,
			ManagerUserID: managerUserID,
		})
		return err
	case "node.move":
		var body struct {
			ID uuid.UUID `json:"id"`
			moveNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		if body.NewParentID == uuid.Nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "new_parent_id is required"}
		}
		_, err = c.org.MoveNode(ctx, tenantID, requestID, initiatorID, services.MoveNodeInput{
			NodeID:        body.ID,
			NewParentID:   body.NewParentID,
			EffectiveDate: effectiveDate,
		})
		return err
	case "node.correct":
		var body struct {
			ID uuid.UUID `json:"id"`
			correctNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		var legalEntityID **uuid.UUID
		if body.LegalEntityID.Set {
			legalEntityID = &body.LegalEntityID.Value
		}
		var companyCode **string
		if body.CompanyCode.Set {
			companyCode = &body.CompanyCode.Value
		}
		var locationID **uuid.UUID
		if body.LocationID.Set {
			locationID = &body.LocationID.Value
		}
		var managerUserID **int64
		if body.ManagerUserID.Set {
			managerUserID = &body.ManagerUserID.Value
		}
		_, err = c.org.CorrectNode(ctx, tenantID, requestID, initiatorID, services.CorrectNodeInput{
			NodeID:        body.ID,
			AsOf:          effectiveDate,
			Name:          body.Name,
			I18nNames:     body.I18nNames,
			Status:        body.Status,
			DisplayOrder:  body.DisplayOrder,
			LegalEntityID: legalEntityID,
			CompanyCode:   companyCode,
			LocationID:    locationID,
			ManagerUserID: managerUserID,
		})
		return err
	case "node.rescind":
		var body struct {
			ID uuid.UUID `json:"id"`
			rescindNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.RescindNode(ctx, tenantID, requestID, initiatorID, services.RescindNodeInput{
			NodeID:        body.ID,
			EffectiveDate: effectiveDate,
			Reason:        body.Reason,
		})
		return err
	case "node.shift_boundary":
		var body struct {
			ID uuid.UUID `json:"id"`
			shiftBoundaryNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		targetDate, err := parseRequiredEffectiveDate(body.TargetEffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "target_effective_date is required", Cause: err}
		}
		newDate, err := parseRequiredEffectiveDate(body.NewEffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "new_effective_date is required", Cause: err}
		}
		_, err = c.org.ShiftBoundaryNode(ctx, tenantID, requestID, initiatorID, services.ShiftBoundaryNodeInput{
			NodeID:              body.ID,
			TargetEffectiveDate: targetDate,
			NewEffectiveDate:    newDate,
		})
		return err
	case "node.correct_move":
		var body struct {
			ID uuid.UUID `json:"id"`
			correctMoveNodeRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		if body.NewParentID == uuid.Nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "new_parent_id is required"}
		}
		_, err = c.org.CorrectMoveNode(ctx, tenantID, requestID, initiatorID, services.CorrectMoveNodeInput{
			NodeID:        body.ID,
			EffectiveDate: effectiveDate,
			NewParentID:   body.NewParentID,
		})
		return err
	case "assignment.create":
		var body createAssignmentRequest
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.CreateAssignment(ctx, tenantID, requestID, initiatorID, services.CreateAssignmentInput{
			Pernr:          body.Pernr,
			EffectiveDate:  effectiveDate,
			PositionID:     body.PositionID,
			OrgNodeID:      body.OrgNodeID,
			AssignmentType: body.AssignmentType,
			SubjectID:      body.SubjectID,
		})
		return err
	case "assignment.update":
		var body struct {
			ID uuid.UUID `json:"id"`
			updateAssignmentRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		var positionID *uuid.UUID
		if body.PositionID.Set {
			positionID = body.PositionID.Value
		}
		var orgNodeID *uuid.UUID
		if body.OrgNodeID.Set {
			orgNodeID = body.OrgNodeID.Value
		}
		_, err = c.org.UpdateAssignment(ctx, tenantID, requestID, initiatorID, services.UpdateAssignmentInput{
			AssignmentID:  body.ID,
			EffectiveDate: effectiveDate,
			PositionID:    positionID,
			OrgNodeID:     orgNodeID,
		})
		return err
	case "assignment.correct":
		var body struct {
			ID uuid.UUID `json:"id"`
			correctAssignmentRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		_, err := c.org.CorrectAssignment(ctx, tenantID, requestID, initiatorID, services.CorrectAssignmentInput{
			AssignmentID: body.ID,
			Pernr:        body.Pernr,
			PositionID:   body.PositionID,
			SubjectID:    body.SubjectID,
		})
		return err
	case "assignment.rescind":
		var body struct {
			ID uuid.UUID `json:"id"`
			rescindAssignmentRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.RescindAssignment(ctx, tenantID, requestID, initiatorID, services.RescindAssignmentInput{
			AssignmentID:  body.ID,
			EffectiveDate: effectiveDate,
			Reason:        body.Reason,
		})
		return err
	default:
		return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "unknown type"}
	}
}

func (c *OrgAPIController) analyzePreflightImpact(ctx context.Context, tenantID uuid.UUID, effective string, commands []batchCommand) (preflightImpact, error) {
	out := preflightImpact{
		Events: map[string]int{
			"org.changed.v1":            0,
			"org.assignment.changed.v1": 0,
		},
		Affected: preflightAffected{
			OrgNodeIDsCount:  0,
			OrgNodeIDsSample: []string{},
		},
	}

	asOf, err := parseEffectiveDate(effective)
	if err != nil || asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	nodes, _, err := c.org.GetHierarchyAsOf(ctx, tenantID, "OrgUnit", asOf)
	if err != nil {
		return preflightImpact{}, err
	}

	children := map[uuid.UUID][]uuid.UUID{}
	for _, n := range nodes {
		if n.ParentID == nil {
			continue
		}
		children[*n.ParentID] = append(children[*n.ParentID], n.ID)
	}

	affected := map[uuid.UUID]struct{}{}
	addAffected := func(id uuid.UUID) {
		if id == uuid.Nil {
			return
		}
		affected[id] = struct{}{}
	}

	const maxSubtree = 5000
	for _, cmd := range commands {
		switch strings.TrimSpace(cmd.Type) {
		case "node.create":
			out.OrgNodes.Create++
			out.Events["org.changed.v1"]++
		case "node.update", "node.correct", "node.shift_boundary":
			out.OrgNodes.Update++
			out.Events["org.changed.v1"]++
			var body struct {
				ID uuid.UUID `json:"id"`
			}
			_ = json.Unmarshal(cmd.Payload, &body)
			addAffected(body.ID)
		case "node.rescind":
			out.OrgNodes.Rescind++
			out.Events["org.changed.v1"]++
			var body struct {
				ID uuid.UUID `json:"id"`
			}
			_ = json.Unmarshal(cmd.Payload, &body)
			addAffected(body.ID)
		case "node.move", "node.correct_move":
			out.OrgNodes.Move++
			out.Events["org.changed.v1"]++
			var body struct {
				ID uuid.UUID `json:"id"`
			}
			_ = json.Unmarshal(cmd.Payload, &body)
			if body.ID != uuid.Nil {
				// Count subtree (including self) at asOf.
				count := 0
				stack := []uuid.UUID{body.ID}
				for len(stack) > 0 {
					n := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					count++
					if count > maxSubtree {
						return preflightImpact{}, &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_TOO_LARGE", Message: "subtree impact is too large"}
					}
					addAffected(n)
					stack = append(stack, children[n]...)
				}
			}
		case "assignment.create":
			out.OrgAssignments.Create++
			out.Events["org.assignment.changed.v1"]++
			var body struct {
				OrgNodeID *uuid.UUID `json:"org_node_id"`
			}
			_ = json.Unmarshal(cmd.Payload, &body)
			if body.OrgNodeID != nil {
				addAffected(*body.OrgNodeID)
			}
		case "assignment.update":
			out.OrgAssignments.Update++
			out.Events["org.assignment.changed.v1"]++
			var body struct {
				OrgNodeID *uuid.UUID `json:"org_node_id"`
			}
			_ = json.Unmarshal(cmd.Payload, &body)
			if body.OrgNodeID != nil {
				addAffected(*body.OrgNodeID)
			}
		case "assignment.correct":
			out.OrgAssignments.Update++
			out.Events["org.assignment.changed.v1"]++
		case "assignment.rescind":
			out.OrgAssignments.Rescind++
			out.Events["org.assignment.changed.v1"]++
		}
	}

	sample := make([]string, 0, 20)
	for id := range affected {
		if len(sample) >= 20 {
			break
		}
		sample = append(sample, id.String())
	}
	out.Affected.OrgNodeIDsCount = len(affected)
	out.Affected.OrgNodeIDsSample = sample
	return out, nil
}

func validateChangeRequestPayload(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("payload is required")
	}
	var payload struct {
		EffectiveDate string         `json:"effective_date"`
		Commands      []batchCommand `json:"commands"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return fmt.Errorf("payload is invalid")
	}
	if len(payload.Commands) < 1 || len(payload.Commands) > 100 {
		return fmt.Errorf("commands size is invalid")
	}
	if strings.TrimSpace(payload.EffectiveDate) != "" {
		if _, err := parseEffectiveDate(payload.EffectiveDate); err != nil {
			return fmt.Errorf("effective_date is invalid")
		}
	}
	for _, cmd := range payload.Commands {
		t := strings.TrimSpace(cmd.Type)
		if t == "" {
			return fmt.Errorf("command type is required")
		}
		if !isSupportedBatchCommandType(t) {
			return fmt.Errorf("unknown command type")
		}
		if len(cmd.Payload) == 0 {
			return fmt.Errorf("command payload is required")
		}
	}
	return nil
}

func isSupportedBatchCommandType(t string) bool {
	switch t {
	case "node.create", "node.update", "node.move", "node.correct", "node.rescind", "node.shift_boundary", "node.correct_move":
		return true
	case "assignment.create", "assignment.update", "assignment.correct", "assignment.rescind":
		return true
	default:
		return false
	}
}

func requireOrgChangeRequestsEnabled(w http.ResponseWriter, requestID string) bool {
	if configuration.Use().OrgChangeRequestsEnabled {
		return true
	}
	writeAPIError(w, http.StatusNotFound, requestID, "ORG_NOT_FOUND", "not found")
	return false
}

func requireOrgPreflightEnabled(w http.ResponseWriter, requestID string) bool {
	if configuration.Use().OrgPreflightEnabled {
		return true
	}
	writeAPIError(w, http.StatusNotFound, requestID, "ORG_NOT_FOUND", "not found")
	return false
}

func withOrgTx[T any](ctx context.Context, tenantID uuid.UUID, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	pool, err := composables.UsePool(ctx)
	if err != nil {
		return zero, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTx(ctx, tx)
	txCtx = composables.WithTenantID(txCtx, tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return zero, err
	}

	out, err := fn(txCtx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return out, nil
}

func parseChangeRequestCursor(raw string) (*time.Time, *uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}
	if !strings.HasPrefix(raw, "updated_at:") {
		return nil, nil, fmt.Errorf("invalid cursor")
	}
	rest := strings.TrimPrefix(raw, "updated_at:")
	atStr, idStr, ok := strings.Cut(rest, ":id:")
	if !ok || strings.TrimSpace(atStr) == "" || strings.TrimSpace(idStr) == "" {
		return nil, nil, fmt.Errorf("invalid cursor")
	}

	at, err := time.Parse(time.RFC3339, atStr)
	if err != nil {
		return nil, nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, nil, err
	}
	at = at.UTC()
	return &at, &id, nil
}

func writePreflightCommandError(w http.ResponseWriter, requestID string, index int, cmdType string, status int, code string, message string) {
	meta := map[string]string{
		"command_index": strconv.Itoa(index),
		"command_type":  cmdType,
	}
	if requestID != "" {
		meta["request_id"] = requestID
	}
	writeJSON(w, status, coredtos.APIError{Code: code, Message: message, Meta: meta})
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
