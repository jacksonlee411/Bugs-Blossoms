package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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

	api.HandleFunc("/assignments", c.GetAssignments).Methods(http.MethodGet)
	api.HandleFunc("/assignments", c.CreateAssignment).Methods(http.MethodPost)
	api.HandleFunc("/assignments/{id}", c.UpdateAssignment).Methods(http.MethodPatch)
}

type effectiveWindowResponse struct {
	EffectiveDate string `json:"effective_date"`
	EndDate       string `json:"end_date"`
}

func (c *OrgAPIController) GetHierarchies(w http.ResponseWriter, r *http.Request) {
	tenantID, requestID, ok := requireSessionAndTenant(w, r)
	if !ok {
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

func (c *OrgAPIController) GetAssignments(w http.ResponseWriter, r *http.Request) {
	tenantID, requestID, ok := requireSessionAndTenant(w, r)
	if !ok {
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
	u, err := composables.UseUser(r.Context())
	if err != nil || u == nil {
		writeAPIError(w, http.StatusUnauthorized, requestID, "ORG_NO_SESSION", "no user")
		return uuid.Nil, nil, requestID, false
	}
	return tenantID, u, requestID, true
}

func ensureRequestID(r *http.Request) string {
	conf := configuration.Use()
	v := strings.TrimSpace(r.Header.Get(conf.RequestIDHeader))
	if v != "" {
		return v
	}
	return uuid.NewString()
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
