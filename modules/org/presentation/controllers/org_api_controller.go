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

	api.HandleFunc("/ops/health", c.instrumentAPI("ops.health.get", c.GetOpsHealth)).Methods(http.MethodGet)

	api.HandleFunc("/hierarchies", c.instrumentAPI("hierarchies.get", c.GetHierarchies)).Methods(http.MethodGet)
	api.HandleFunc("/hierarchies:export", c.instrumentAPI("hierarchies.export.get", c.ExportHierarchies)).Methods(http.MethodGet)

	api.HandleFunc("/nodes", c.instrumentAPI("nodes.create", c.CreateNode)).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}", c.instrumentAPI("nodes.update", c.UpdateNode)).Methods(http.MethodPatch)
	api.HandleFunc("/nodes/{id}:path", c.instrumentAPI("nodes.path.get", c.GetNodePath)).Methods(http.MethodGet)
	api.HandleFunc("/nodes/{id}:resolved-attributes", c.instrumentAPI("nodes.resolved_attributes.get", c.GetNodeResolvedAttributes)).Methods(http.MethodGet)
	api.HandleFunc("/nodes/{id}:move", c.instrumentAPI("nodes.move", c.MoveNode)).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:correct", c.instrumentAPI("nodes.correct", c.CorrectNode)).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:rescind", c.instrumentAPI("nodes.rescind", c.RescindNode)).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:shift-boundary", c.instrumentAPI("nodes.shift_boundary", c.ShiftBoundaryNode)).Methods(http.MethodPost)
	api.HandleFunc("/nodes/{id}:correct-move", c.instrumentAPI("nodes.correct_move", c.CorrectMoveNode)).Methods(http.MethodPost)

	api.HandleFunc("/positions", c.instrumentAPI("positions.list", c.GetPositions)).Methods(http.MethodGet)
	api.HandleFunc("/positions", c.instrumentAPI("positions.create", c.CreatePosition)).Methods(http.MethodPost)
	api.HandleFunc("/positions/{id}", c.instrumentAPI("positions.get", c.GetPosition)).Methods(http.MethodGet)
	api.HandleFunc("/positions/{id}/timeline", c.instrumentAPI("positions.timeline", c.GetPositionTimeline)).Methods(http.MethodGet)
	api.HandleFunc("/positions/{id}", c.instrumentAPI("positions.update", c.UpdatePosition)).Methods(http.MethodPatch)
	api.HandleFunc("/positions/{id}:correct", c.instrumentAPI("positions.correct", c.CorrectPosition)).Methods(http.MethodPost)
	api.HandleFunc("/positions/{id}:rescind", c.instrumentAPI("positions.rescind", c.RescindPosition)).Methods(http.MethodPost)
	api.HandleFunc("/positions/{id}:shift-boundary", c.instrumentAPI("positions.shift_boundary", c.ShiftBoundaryPosition)).Methods(http.MethodPost)
	api.HandleFunc("/positions/{id}/restrictions", c.instrumentAPI("positions.restrictions.get", c.GetPositionRestrictions)).Methods(http.MethodGet)
	api.HandleFunc("/positions/{id}:set-restrictions", c.instrumentAPI("positions.restrictions.set", c.SetPositionRestrictions)).Methods(http.MethodPost)

	api.HandleFunc("/job-catalog/family-groups", c.instrumentAPI("job_catalog.family_groups.list", c.ListJobFamilyGroups)).Methods(http.MethodGet)
	api.HandleFunc("/job-catalog/family-groups", c.instrumentAPI("job_catalog.family_groups.create", c.CreateJobFamilyGroup)).Methods(http.MethodPost)
	api.HandleFunc("/job-catalog/family-groups/{id}", c.instrumentAPI("job_catalog.family_groups.update", c.UpdateJobFamilyGroup)).Methods(http.MethodPatch)

	api.HandleFunc("/job-catalog/families", c.instrumentAPI("job_catalog.families.list", c.ListJobFamilies)).Methods(http.MethodGet)
	api.HandleFunc("/job-catalog/families", c.instrumentAPI("job_catalog.families.create", c.CreateJobFamily)).Methods(http.MethodPost)
	api.HandleFunc("/job-catalog/families/{id}", c.instrumentAPI("job_catalog.families.update", c.UpdateJobFamily)).Methods(http.MethodPatch)

	api.HandleFunc("/job-catalog/roles", c.instrumentAPI("job_catalog.roles.list", c.ListJobRoles)).Methods(http.MethodGet)
	api.HandleFunc("/job-catalog/roles", c.instrumentAPI("job_catalog.roles.create", c.CreateJobRole)).Methods(http.MethodPost)
	api.HandleFunc("/job-catalog/roles/{id}", c.instrumentAPI("job_catalog.roles.update", c.UpdateJobRole)).Methods(http.MethodPatch)

	api.HandleFunc("/job-catalog/levels", c.instrumentAPI("job_catalog.levels.list", c.ListJobLevels)).Methods(http.MethodGet)
	api.HandleFunc("/job-catalog/levels", c.instrumentAPI("job_catalog.levels.create", c.CreateJobLevel)).Methods(http.MethodPost)
	api.HandleFunc("/job-catalog/levels/{id}", c.instrumentAPI("job_catalog.levels.update", c.UpdateJobLevel)).Methods(http.MethodPatch)

	api.HandleFunc("/job-profiles", c.instrumentAPI("job_profiles.list", c.ListJobProfiles)).Methods(http.MethodGet)
	api.HandleFunc("/job-profiles", c.instrumentAPI("job_profiles.create", c.CreateJobProfile)).Methods(http.MethodPost)
	api.HandleFunc("/job-profiles/{id}", c.instrumentAPI("job_profiles.update", c.UpdateJobProfile)).Methods(http.MethodPatch)
	api.HandleFunc("/job-profiles/{id}:set-allowed-levels", c.instrumentAPI("job_profiles.allowed_levels.set", c.SetJobProfileAllowedLevels)).Methods(http.MethodPost)

	api.HandleFunc("/assignments", c.instrumentAPI("assignments.list", c.GetAssignments)).Methods(http.MethodGet)
	api.HandleFunc("/assignments", c.instrumentAPI("assignments.create", c.CreateAssignment)).Methods(http.MethodPost)
	api.HandleFunc("/assignments/{id}", c.instrumentAPI("assignments.update", c.UpdateAssignment)).Methods(http.MethodPatch)
	api.HandleFunc("/assignments/{id}:correct", c.instrumentAPI("assignments.correct", c.CorrectAssignment)).Methods(http.MethodPost)
	api.HandleFunc("/assignments/{id}:rescind", c.instrumentAPI("assignments.rescind", c.RescindAssignment)).Methods(http.MethodPost)

	api.HandleFunc("/personnel-events/hire", c.instrumentAPI("personnel_events.hire", c.HirePersonnelEvent)).Methods(http.MethodPost)
	api.HandleFunc("/personnel-events/transfer", c.instrumentAPI("personnel_events.transfer", c.TransferPersonnelEvent)).Methods(http.MethodPost)
	api.HandleFunc("/personnel-events/termination", c.instrumentAPI("personnel_events.termination", c.TerminationPersonnelEvent)).Methods(http.MethodPost)

	api.HandleFunc("/roles", c.instrumentAPI("roles.list", c.GetRoles)).Methods(http.MethodGet)
	api.HandleFunc("/role-assignments", c.instrumentAPI("role_assignments.list", c.GetRoleAssignments)).Methods(http.MethodGet)

	api.HandleFunc("/security-group-mappings", c.instrumentAPI("security_group_mappings.list", c.GetSecurityGroupMappings)).Methods(http.MethodGet)
	api.HandleFunc("/security-group-mappings", c.instrumentAPI("security_group_mappings.create", c.CreateSecurityGroupMapping)).Methods(http.MethodPost)
	api.HandleFunc("/security-group-mappings/{id}:rescind", c.instrumentAPI("security_group_mappings.rescind", c.RescindSecurityGroupMapping)).Methods(http.MethodPost)

	api.HandleFunc("/links", c.instrumentAPI("links.list", c.GetLinks)).Methods(http.MethodGet)
	api.HandleFunc("/links", c.instrumentAPI("links.create", c.CreateLink)).Methods(http.MethodPost)
	api.HandleFunc("/links/{id}:rescind", c.instrumentAPI("links.rescind", c.RescindLink)).Methods(http.MethodPost)

	api.HandleFunc("/permission-preview", c.instrumentAPI("permission_preview.get", c.GetPermissionPreview)).Methods(http.MethodGet)

	api.HandleFunc("/snapshot", c.instrumentAPI("snapshot.get", c.GetSnapshot)).Methods(http.MethodGet)
	api.HandleFunc("/batch", c.instrumentAPI("batch.post", c.Batch)).Methods(http.MethodPost)
	api.HandleFunc("/reports/person-path", c.instrumentAPI("reports.person_path.get", c.GetPersonPath)).Methods(http.MethodGet)
	api.HandleFunc("/reports/staffing:summary", c.instrumentAPI("reports.staffing_summary.get", c.GetStaffingSummary)).Methods(http.MethodGet)
	api.HandleFunc("/reports/staffing:vacancies", c.instrumentAPI("reports.staffing_vacancies.get", c.GetStaffingVacancies)).Methods(http.MethodGet)
	api.HandleFunc("/reports/staffing:time-to-fill", c.instrumentAPI("reports.staffing_time_to_fill.get", c.GetStaffingTimeToFill)).Methods(http.MethodGet)
	api.HandleFunc("/reports/staffing:export", c.instrumentAPI("reports.staffing_export.get", c.ExportStaffingReport)).Methods(http.MethodGet)

	api.HandleFunc("/change-requests", c.instrumentAPI("change_requests.create", c.CreateChangeRequest)).Methods(http.MethodPost)
	api.HandleFunc("/change-requests", c.instrumentAPI("change_requests.list", c.ListChangeRequests)).Methods(http.MethodGet)
	api.HandleFunc("/change-requests/{id}", c.instrumentAPI("change_requests.get", c.GetChangeRequest)).Methods(http.MethodGet)
	api.HandleFunc("/change-requests/{id}", c.instrumentAPI("change_requests.update", c.UpdateChangeRequest)).Methods(http.MethodPatch)
	api.HandleFunc("/change-requests/{id}:submit", c.instrumentAPI("change_requests.submit", c.SubmitChangeRequest)).Methods(http.MethodPost)
	api.HandleFunc("/change-requests/{id}:cancel", c.instrumentAPI("change_requests.cancel", c.CancelChangeRequest)).Methods(http.MethodPost)

	api.HandleFunc("/preflight", c.instrumentAPI("preflight.post", c.Preflight)).Methods(http.MethodPost)
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

	include := strings.TrimSpace(r.URL.Query().Get("include"))
	includeResolved := false
	if include != "" {
		parts := strings.Split(include, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if p == "resolved_attributes" {
				includeResolved = true
				continue
			}
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include is invalid")
			return
		}
	}

	if includeResolved {
		if hType != "OrgUnit" {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "type is invalid")
			return
		}

		nodes, effectiveDate, _, _, err := c.org.GetHierarchyResolvedAttributes(r.Context(), tenantID, hType, asOf)
		if err != nil {
			writeServiceError(w, requestID, err)
			return
		}

		type hierarchiesResolvedResponse struct {
			TenantID      string                                         `json:"tenant_id"`
			HierarchyType string                                         `json:"hierarchy_type"`
			EffectiveDate string                                         `json:"effective_date"`
			Nodes         []services.HierarchyNodeWithResolvedAttributes `json:"nodes"`
		}
		writeJSON(w, http.StatusOK, hierarchiesResolvedResponse{
			TenantID:      tenantID.String(),
			HierarchyType: hType,
			EffectiveDate: formatValidDate(effectiveDate),
			Nodes:         nodes,
		})
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
		EffectiveDate: formatValidDate(effectiveDate),
		Nodes:         nodes,
	})
}

func (c *OrgAPIController) ExportHierarchies(w http.ResponseWriter, r *http.Request) {
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

	var rootNodeID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("root_node_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "root_node_id is invalid")
			return
		}
		rootNodeID = &id
	}

	var maxDepth *int
	if raw := strings.TrimSpace(r.URL.Query().Get("max_depth")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "max_depth is invalid")
			return
		}
		maxDepth = &n
	}

	includeEdges := false
	includeSecurityGroups := false
	includeLinks := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			switch part {
			case "nodes":
			case "edges":
				includeEdges = true
			case "security_groups":
				includeSecurityGroups = true
			case "links":
				includeLinks = true
			default:
				writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include is invalid")
				return
			}
		}
	}

	if includeSecurityGroups && !requireOrgSecurityGroupMappingsEnabled(w, requestID) {
		return
	}
	if includeLinks && !requireOrgLinksEnabled(w, requestID) {
		return
	}

	limit := 2000
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = n
	}

	afterID, afterOK, err := parseIDCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
		return
	}
	var afterPtr *uuid.UUID
	if afterOK {
		afterPtr = &afterID
	}

	res, err := c.org.ExportHierarchy(
		r.Context(),
		tenantID,
		hType,
		asOf,
		rootNodeID,
		maxDepth,
		includeEdges,
		includeSecurityGroups,
		includeLinks,
		limit,
		afterPtr,
	)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type exportNode struct {
		ID                string                    `json:"id"`
		ParentID          *string                   `json:"parent_id"`
		Code              string                    `json:"code"`
		Name              string                    `json:"name"`
		Depth             int                       `json:"depth"`
		Status            string                    `json:"status"`
		SecurityGroupKeys []string                  `json:"security_group_keys,omitempty"`
		Links             []services.OrgLinkSummary `json:"links,omitempty"`
	}
	nodes := make([]exportNode, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		var pid *string
		if n.ParentID != nil && *n.ParentID != uuid.Nil {
			v := n.ParentID.String()
			pid = &v
		}
		nodes = append(nodes, exportNode{
			ID:                n.ID.String(),
			ParentID:          pid,
			Code:              n.Code,
			Name:              n.Name,
			Depth:             n.Depth,
			Status:            n.Status,
			SecurityGroupKeys: n.SecurityGroupKeys,
			Links:             n.Links,
		})
	}

	type exportEdge struct {
		ChildNodeID  string  `json:"child_node_id"`
		ParentNodeID *string `json:"parent_node_id"`
	}
	edges := []exportEdge(nil)
	if includeEdges {
		edges = make([]exportEdge, 0, len(nodes))
		for _, n := range nodes {
			edges = append(edges, exportEdge{ChildNodeID: n.ID, ParentNodeID: n.ParentID})
		}
	}

	var nextCursor *string
	if res.NextCursorID != nil && *res.NextCursorID != uuid.Nil {
		v := "id:" + res.NextCursorID.String()
		nextCursor = &v
	}

	type exportResponse struct {
		TenantID      string       `json:"tenant_id"`
		HierarchyType string       `json:"hierarchy_type"`
		EffectiveDate string       `json:"effective_date"`
		RootNodeID    string       `json:"root_node_id"`
		Includes      []string     `json:"includes"`
		Limit         int          `json:"limit"`
		Nodes         []exportNode `json:"nodes"`
		Edges         []exportEdge `json:"edges,omitempty"`
		NextCursor    *string      `json:"next_cursor"`
	}
	writeJSON(w, http.StatusOK, exportResponse{
		TenantID:      res.TenantID.String(),
		HierarchyType: res.HierarchyType,
		EffectiveDate: formatValidDate(res.EffectiveDate),
		RootNodeID:    res.RootNodeID.String(),
		Includes:      res.Includes,
		Limit:         res.Limit,
		Nodes:         nodes,
		Edges:         edges,
		NextCursor:    nextCursor,
	})
}

func (c *OrgAPIController) GetNodePath(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	idRaw := mux.Vars(r)["id"]
	orgNodeID, err := uuid.Parse(idRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "id is invalid")
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		format = "nodes"
	}
	if format != "nodes" && format != "nodes_with_sources" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "format is invalid")
		return
	}

	res, err := c.org.GetNodePath(r.Context(), tenantID, orgNodeID, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type nodePathNode struct {
		ID    string `json:"id"`
		Code  string `json:"code"`
		Name  string `json:"name"`
		Depth int    `json:"depth"`
	}
	type nodePathSource struct {
		DeepReadBackend string `json:"deep_read_backend"`
		AsOfDate        string `json:"as_of_date"`
	}
	type nodePathResponse struct {
		TenantID      string `json:"tenant_id"`
		OrgNodeID     string `json:"org_node_id"`
		EffectiveDate string `json:"effective_date"`
		Path          struct {
			Nodes []nodePathNode `json:"nodes"`
		} `json:"path"`
		Source *nodePathSource `json:"source,omitempty"`
	}

	nodes := make([]nodePathNode, 0, len(res.Path))
	for _, n := range res.Path {
		nodes = append(nodes, nodePathNode{
			ID:    n.ID.String(),
			Code:  n.Code,
			Name:  n.Name,
			Depth: n.Depth,
		})
	}

	var out nodePathResponse
	out.TenantID = res.TenantID.String()
	out.OrgNodeID = res.OrgNodeID.String()
	out.EffectiveDate = formatValidDate(res.EffectiveDate)
	out.Path.Nodes = nodes
	if format == "nodes_with_sources" {
		out.Source = &nodePathSource{
			DeepReadBackend: string(res.Source.DeepReadBackend),
			AsOfDate:        res.Source.AsOfDate,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (c *OrgAPIController) GetPersonPath(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "read") {
		return
	}

	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "subject is required")
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	res, err := c.org.GetPersonPath(r.Context(), tenantID, subject, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type personPathNode struct {
		ID    string `json:"id"`
		Code  string `json:"code"`
		Name  string `json:"name"`
		Depth int    `json:"depth"`
	}
	type personPathResponse struct {
		TenantID      string `json:"tenant_id"`
		Subject       string `json:"subject"`
		EffectiveDate string `json:"effective_date"`
		Assignment    struct {
			AssignmentID string `json:"assignment_id"`
			PositionID   string `json:"position_id"`
			OrgNodeID    string `json:"org_node_id"`
		} `json:"assignment"`
		Path struct {
			Nodes []personPathNode `json:"nodes"`
		} `json:"path"`
	}
	nodes := make([]personPathNode, 0, len(res.Path))
	for _, n := range res.Path {
		nodes = append(nodes, personPathNode{
			ID:    n.ID.String(),
			Code:  n.Code,
			Name:  n.Name,
			Depth: n.Depth,
		})
	}

	var out personPathResponse
	out.TenantID = res.TenantID.String()
	out.Subject = res.Subject
	out.EffectiveDate = formatValidDate(res.EffectiveDate)
	out.Assignment.AssignmentID = res.Assignment.AssignmentID.String()
	out.Assignment.PositionID = res.Assignment.PositionID.String()
	out.Assignment.OrgNodeID = res.Assignment.OrgNodeID.String()
	out.Path.Nodes = nodes

	writeJSON(w, http.StatusOK, out)
}

func (c *OrgAPIController) GetNodeResolvedAttributes(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	nodeID, err := uuid.Parse(strings.TrimSpace(mux.Vars(r)["id"]))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "id is invalid")
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	var requested []string
	attrRaw := strings.TrimSpace(r.URL.Query().Get("attributes"))
	if attrRaw != "" {
		seen := map[string]struct{}{}
		for _, part := range strings.Split(attrRaw, ",") {
			name := strings.TrimSpace(strings.ToLower(part))
			if name == "" {
				continue
			}
			if !isOrgInheritanceAttributeWhitelisted(name) {
				writeAPIError(w, http.StatusBadRequest, requestID, "ORG_UNKNOWN_ATTRIBUTE", "unknown attribute")
				return
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			requested = append(requested, name)
		}
	}

	const hierarchyType = "OrgUnit"
	nodes, effectiveDate, sourcesByNode, ruleAttrs, err := c.org.GetHierarchyResolvedAttributes(r.Context(), tenantID, hierarchyType, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	var node services.HierarchyNodeWithResolvedAttributes
	found := false
	for _, n := range nodes {
		if n.ID == nodeID {
			node = n
			found = true
			break
		}
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, requestID, "ORG_NODE_NOT_FOUND_AT_DATE", "org_node_id not found at effective_date")
		return
	}

	selected := requested
	if len(selected) == 0 {
		selected = ruleAttrs
	}

	attrs := map[string]any{}
	resolved := map[string]any{}
	resolvedSources := map[string]*uuid.UUID{}

	sources := sourcesByNode[nodeID]
	for _, name := range selected {
		switch name {
		case "legal_entity_id":
			attrs[name] = node.Attributes.LegalEntityID
			resolved[name] = node.ResolvedAttributes.LegalEntityID
			resolvedSources[name] = sources.LegalEntityID
		case "company_code":
			attrs[name] = node.Attributes.CompanyCode
			resolved[name] = node.ResolvedAttributes.CompanyCode
			resolvedSources[name] = sources.CompanyCode
		case "location_id":
			attrs[name] = node.Attributes.LocationID
			resolved[name] = node.ResolvedAttributes.LocationID
			resolvedSources[name] = sources.LocationID
		case "manager_user_id":
			attrs[name] = node.Attributes.ManagerUserID
			resolved[name] = node.ResolvedAttributes.ManagerUserID
			resolvedSources[name] = sources.ManagerUserID
		default:
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_UNKNOWN_ATTRIBUTE", "unknown attribute")
			return
		}
	}

	type response struct {
		TenantID           string                `json:"tenant_id"`
		HierarchyType      string                `json:"hierarchy_type"`
		OrgNodeID          string                `json:"org_node_id"`
		EffectiveDate      string                `json:"effective_date"`
		Attributes         map[string]any        `json:"attributes"`
		ResolvedAttributes map[string]any        `json:"resolved_attributes"`
		ResolvedSources    map[string]*uuid.UUID `json:"resolved_sources"`
	}
	writeJSON(w, http.StatusOK, response{
		TenantID:           tenantID.String(),
		HierarchyType:      hierarchyType,
		OrgNodeID:          nodeID.String(),
		EffectiveDate:      formatValidDate(effectiveDate),
		Attributes:         attrs,
		ResolvedAttributes: resolved,
		ResolvedSources:    resolvedSources,
	})
}

func isOrgInheritanceAttributeWhitelisted(name string) bool {
	switch name {
	case "legal_entity_id", "company_code", "location_id", "manager_user_id":
		return true
	default:
		return false
	}
}

func (c *OrgAPIController) GetRoles(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgRolesAuthzObject, "admin") {
		return
	}

	roles, err := c.org.ListRoles(r.Context(), tenantID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		TenantID string             `json:"tenant_id"`
		Roles    []services.OrgRole `json:"roles"`
	}
	writeJSON(w, http.StatusOK, response{
		TenantID: tenantID.String(),
		Roles:    roles,
	})
}

func (c *OrgAPIController) GetRoleAssignments(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgRoleAssignmentsAuthzObject, "admin") {
		return
	}

	orgNodeRaw := strings.TrimSpace(r.URL.Query().Get("org_node_id"))
	if orgNodeRaw == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is required")
		return
	}
	orgNodeID, err := uuid.Parse(orgNodeRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
		return
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}

	includeInherited := false
	if raw := strings.TrimSpace(r.URL.Query().Get("include_inherited")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include_inherited is invalid")
			return
		}
		includeInherited = v
	}

	var roleCode *string
	if raw := strings.TrimSpace(r.URL.Query().Get("role")); raw != "" {
		roleCode = &raw
	}

	var subjectType *string
	var subjectID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("subject")); raw != "" {
		parts := strings.Split(raw, ":")
		if len(parts) != 2 {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "subject is invalid")
			return
		}
		st := strings.TrimSpace(strings.ToLower(parts[0]))
		sid, err := uuid.Parse(strings.TrimSpace(parts[1]))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "subject is invalid")
			return
		}
		if st != "user" && st != "group" {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "subject is invalid")
			return
		}
		subjectType = &st
		subjectID = &sid
	}

	items, effectiveDate, err := c.org.ListRoleAssignments(r.Context(), tenantID, orgNodeID, asOf, includeInherited, roleCode, subjectType, subjectID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		TenantID         string                        `json:"tenant_id"`
		OrgNodeID        string                        `json:"org_node_id"`
		EffectiveDate    string                        `json:"effective_date"`
		IncludeInherited bool                          `json:"include_inherited"`
		Items            []services.RoleAssignmentItem `json:"items"`
	}
	writeJSON(w, http.StatusOK, response{
		TenantID:         tenantID.String(),
		OrgNodeID:        orgNodeID.String(),
		EffectiveDate:    formatValidDate(effectiveDate),
		IncludeInherited: includeInherited,
		Items:            items,
	})
}

type securityGroupMappingItem struct {
	ID               string                  `json:"id"`
	OrgNodeID        string                  `json:"org_node_id"`
	SecurityGroupKey string                  `json:"security_group_key"`
	AppliesToSubtree bool                    `json:"applies_to_subtree"`
	EffectiveWindow  effectiveWindowResponse `json:"effective_window"`
}

type listSecurityGroupMappingsResponse struct {
	TenantID      string                     `json:"tenant_id"`
	EffectiveDate *string                    `json:"effective_date"`
	Items         []securityGroupMappingItem `json:"items"`
	NextCursor    *string                    `json:"next_cursor"`
}

func (c *OrgAPIController) GetSecurityGroupMappings(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgSecurityGroupMappingsObj, "admin") {
		return
	}

	var orgNodeID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("org_node_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
			return
		}
		orgNodeID = &id
	}

	var securityGroupKey *string
	if raw := strings.TrimSpace(r.URL.Query().Get("security_group_key")); raw != "" {
		securityGroupKey = &raw
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}
	var asOfPtr *time.Time
	if !asOf.IsZero() {
		asOfPtr = &asOf
	}

	limit := 200
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
	if limit > 1000 {
		limit = 1000
	}

	var cursorAt *time.Time
	var cursorID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		at, id, err := parseEffectiveIDCursor(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
			return
		}
		cursorAt, cursorID = at, id
	}

	res, err := c.org.ListSecurityGroupMappings(r.Context(), tenantID, services.SecurityGroupMappingListFilter{
		OrgNodeID:        orgNodeID,
		SecurityGroupKey: securityGroupKey,
		AsOf:             asOfPtr,
		Limit:            limit,
		CursorAt:         cursorAt,
		CursorID:         cursorID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	items := make([]securityGroupMappingItem, 0, len(res.Items))
	for _, row := range res.Items {
		items = append(items, securityGroupMappingItem{
			ID:               row.ID.String(),
			OrgNodeID:        row.OrgNodeID.String(),
			SecurityGroupKey: row.SecurityGroupKey,
			AppliesToSubtree: row.AppliesToSubtree,
			EffectiveWindow: effectiveWindowResponse{
				EffectiveDate: formatValidDate(row.EffectiveDate),
				EndDate:       formatValidEndDateFromEndDate(row.EndDate),
			},
		})
	}

	var ed *string
	if res.EffectiveDate != nil && !res.EffectiveDate.IsZero() {
		v := formatValidDate(*res.EffectiveDate)
		ed = &v
	}

	writeJSON(w, http.StatusOK, listSecurityGroupMappingsResponse{
		TenantID:      tenantID.String(),
		EffectiveDate: ed,
		Items:         items,
		NextCursor:    res.NextCursor,
	})
}

type createSecurityGroupMappingRequest struct {
	OrgNodeID        uuid.UUID `json:"org_node_id"`
	SecurityGroupKey string    `json:"security_group_key"`
	AppliesToSubtree bool      `json:"applies_to_subtree"`
	EffectiveDate    string    `json:"effective_date"`
}

func (c *OrgAPIController) CreateSecurityGroupMapping(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgSecurityGroupMappingsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgSecurityGroupMappingsObj, "admin") {
		return
	}

	var req createSecurityGroupMappingRequest
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
	res, err := c.org.CreateSecurityGroupMapping(r.Context(), tenantID, requestID, initiatorID, services.CreateSecurityGroupMappingInput{
		OrgNodeID:        req.OrgNodeID,
		SecurityGroupKey: req.SecurityGroupKey,
		AppliesToSubtree: req.AppliesToSubtree,
		EffectiveDate:    effectiveDate,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusCreated, response{
		ID: res.ID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type rescindSecurityGroupMappingRequest struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
}

func (c *OrgAPIController) RescindSecurityGroupMapping(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgSecurityGroupMappingsEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgSecurityGroupMappingsObj, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req rescindSecurityGroupMappingRequest
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
	res, err := c.org.RescindSecurityGroupMapping(r.Context(), tenantID, requestID, initiatorID, services.RescindSecurityGroupMappingInput{
		ID:            id,
		EffectiveDate: effectiveDate,
		Reason:        req.Reason,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, response{
		ID: res.ID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type orgLinkItem struct {
	ID              string                  `json:"id"`
	OrgNodeID       string                  `json:"org_node_id"`
	ObjectType      string                  `json:"object_type"`
	ObjectKey       string                  `json:"object_key"`
	LinkType        string                  `json:"link_type"`
	Metadata        json.RawMessage         `json:"metadata"`
	EffectiveWindow effectiveWindowResponse `json:"effective_window"`
}

type listOrgLinksResponse struct {
	TenantID      string        `json:"tenant_id"`
	EffectiveDate *string       `json:"effective_date"`
	Items         []orgLinkItem `json:"items"`
	NextCursor    *string       `json:"next_cursor"`
}

func (c *OrgAPIController) GetLinks(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgLinksAuthzObject, "admin") {
		return
	}

	var orgNodeID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("org_node_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
			return
		}
		orgNodeID = &id
	}

	var objectType *string
	if raw := strings.TrimSpace(r.URL.Query().Get("object_type")); raw != "" {
		objectType = &raw
	}
	var objectKey *string
	if raw := strings.TrimSpace(r.URL.Query().Get("object_key")); raw != "" {
		objectKey = &raw
	}

	asOf, err := parseEffectiveDate(r.URL.Query().Get("effective_date"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "effective_date is invalid")
		return
	}
	var asOfPtr *time.Time
	if !asOf.IsZero() {
		asOfPtr = &asOf
	}

	limit := 200
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
	if limit > 1000 {
		limit = 1000
	}

	var cursorAt *time.Time
	var cursorID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		at, id, err := parseEffectiveIDCursor(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "cursor is invalid")
			return
		}
		cursorAt, cursorID = at, id
	}

	res, err := c.org.ListOrgLinks(r.Context(), tenantID, services.OrgLinkListFilter{
		OrgNodeID:  orgNodeID,
		ObjectType: objectType,
		ObjectKey:  objectKey,
		AsOf:       asOfPtr,
		Limit:      limit,
		CursorAt:   cursorAt,
		CursorID:   cursorID,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	items := make([]orgLinkItem, 0, len(res.Items))
	for _, row := range res.Items {
		items = append(items, orgLinkItem{
			ID:         row.ID.String(),
			OrgNodeID:  row.OrgNodeID.String(),
			ObjectType: row.ObjectType,
			ObjectKey:  row.ObjectKey,
			LinkType:   row.LinkType,
			Metadata:   row.Metadata,
			EffectiveWindow: effectiveWindowResponse{
				EffectiveDate: formatValidDate(row.EffectiveDate),
				EndDate:       formatValidEndDateFromEndDate(row.EndDate),
			},
		})
	}

	var ed *string
	if res.EffectiveDate != nil && !res.EffectiveDate.IsZero() {
		v := formatValidDate(*res.EffectiveDate)
		ed = &v
	}

	writeJSON(w, http.StatusOK, listOrgLinksResponse{
		TenantID:      tenantID.String(),
		EffectiveDate: ed,
		Items:         items,
		NextCursor:    res.NextCursor,
	})
}

type createLinkRequest struct {
	OrgNodeID     uuid.UUID      `json:"org_node_id"`
	ObjectType    string         `json:"object_type"`
	ObjectKey     string         `json:"object_key"`
	LinkType      string         `json:"link_type"`
	Metadata      map[string]any `json:"metadata"`
	EffectiveDate string         `json:"effective_date"`
}

func (c *OrgAPIController) CreateLink(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgLinksEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgLinksAuthzObject, "admin") {
		return
	}

	var req createLinkRequest
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
	res, err := c.org.CreateOrgLink(r.Context(), tenantID, requestID, initiatorID, services.CreateOrgLinkInput{
		OrgNodeID:     req.OrgNodeID,
		ObjectType:    req.ObjectType,
		ObjectKey:     req.ObjectKey,
		LinkType:      req.LinkType,
		Metadata:      req.Metadata,
		EffectiveDate: effectiveDate,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusCreated, response{
		ID: res.ID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type rescindLinkRequest struct {
	EffectiveDate string `json:"effective_date"`
	Reason        string `json:"reason"`
}

func (c *OrgAPIController) RescindLink(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgLinksEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgLinksAuthzObject, "admin") {
		return
	}

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req rescindLinkRequest
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
	res, err := c.org.RescindOrgLink(r.Context(), tenantID, requestID, initiatorID, services.RescindOrgLinkInput{
		ID:            id,
		EffectiveDate: effectiveDate,
		Reason:        req.Reason,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type response struct {
		ID              string                  `json:"id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, response{
		ID: res.ID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type permissionPreviewSecurityGroupItem struct {
	SecurityGroupKey string `json:"security_group_key"`
	AppliesToSubtree bool   `json:"applies_to_subtree"`
	SourceOrgNodeID  string `json:"source_org_node_id"`
	SourceDepth      int    `json:"source_depth"`
}

type permissionPreviewResponse struct {
	TenantID       string                               `json:"tenant_id"`
	OrgNodeID      string                               `json:"org_node_id"`
	EffectiveDate  string                               `json:"effective_date"`
	SecurityGroups []permissionPreviewSecurityGroupItem `json:"security_groups"`
	Links          []services.PermissionPreviewLink     `json:"links"`
	Warnings       []string                             `json:"warnings"`
}

func (c *OrgAPIController) GetPermissionPreview(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !requireOrgPermissionPreviewEnabled(w, requestID) {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPermissionPreviewAuthzObj, "admin") {
		return
	}

	orgNodeRaw := strings.TrimSpace(r.URL.Query().Get("org_node_id"))
	if orgNodeRaw == "" {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is required")
		return
	}
	orgNodeID, err := uuid.Parse(orgNodeRaw)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
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

	includeRaw := strings.TrimSpace(r.URL.Query().Get("include"))
	includeSecurityGroups := true
	includeLinks := true
	if includeRaw != "" {
		includeSecurityGroups = false
		includeLinks = false
		for _, part := range strings.Split(includeRaw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			switch part {
			case "security_groups":
				includeSecurityGroups = true
			case "links":
				includeLinks = true
			default:
				writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "include is invalid")
				return
			}
		}
	}

	limitLinks := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit_links")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit_links is invalid")
			return
		}
		limitLinks = n
	}
	if limitLinks < 1 {
		limitLinks = 1
	}
	if limitLinks > 1000 {
		limitLinks = 1000
	}

	res, err := c.org.PermissionPreview(r.Context(), tenantID, services.PermissionPreviewInput{
		OrgNodeID:             orgNodeID,
		EffectiveDate:         asOf,
		IncludeSecurityGroups: includeSecurityGroups,
		IncludeLinks:          includeLinks,
		LimitLinks:            limitLinks,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	sg := make([]permissionPreviewSecurityGroupItem, 0, len(res.SecurityGroups))
	for _, row := range res.SecurityGroups {
		sg = append(sg, permissionPreviewSecurityGroupItem{
			SecurityGroupKey: row.SecurityGroupKey,
			AppliesToSubtree: row.AppliesToSubtree,
			SourceOrgNodeID:  row.SourceOrgNodeID.String(),
			SourceDepth:      row.SourceDepth,
		})
	}

	writeJSON(w, http.StatusOK, permissionPreviewResponse{
		TenantID:       res.TenantID.String(),
		OrgNodeID:      res.OrgNodeID.String(),
		EffectiveDate:  formatValidDate(res.EffectiveDate),
		SecurityGroups: sg,
		Links:          res.Links,
		Warnings:       res.Warnings,
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
	target, err := parseRequiredValidDate("target_effective_date", req.TargetEffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "target_effective_date is required")
		return
	}
	newStart, err := parseRequiredValidDate("new_effective_date", req.NewEffectiveDate)
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
	resp.Shifted.TargetEffectiveDate = formatValidDate(res.TargetStart)
	resp.Shifted.NewEffectiveDate = formatValidDate(res.NewStart)
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
		EffectiveDate: formatValidDate(res.EffectiveDate),
	})
}

func (c *OrgAPIController) GetPositions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
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

	var orgNodeID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("org_node_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "org_node_id is invalid")
			return
		}
		orgNodeID = &id
	}

	var q *string
	if raw := strings.TrimSpace(r.URL.Query().Get("q")); raw != "" {
		q = &raw
	}

	var lifecycleStatus *string
	if raw := strings.TrimSpace(r.URL.Query().Get("lifecycle_status")); raw != "" {
		lifecycleStatus = &raw
	}

	var isAutoCreated *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("is_auto_created")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "is_auto_created is invalid")
			return
		}
		isAutoCreated = &v
	}

	limit := 25
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "limit is invalid")
			return
		}
		limit = v
	}
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "page is invalid")
			return
		}
		page = v
	}
	offset := (page - 1) * limit

	rows, effectiveDate, err := c.org.GetPositions(r.Context(), tenantID, services.GetPositionsInput{
		AsOf:            asOf,
		OrgNodeID:       orgNodeID,
		Q:               q,
		LifecycleStatus: lifecycleStatus,
		IsAutoCreated:   isAutoCreated,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type positionViewRowResponse struct {
		PositionID          uuid.UUID       `json:"position_id"`
		Code                string          `json:"code"`
		OrgNodeID           uuid.UUID       `json:"org_node_id"`
		Title               *string         `json:"title,omitempty"`
		LifecycleStatus     string          `json:"lifecycle_status"`
		IsAutoCreated       bool            `json:"is_auto_created"`
		CapacityFTE         float64         `json:"capacity_fte"`
		OccupiedFTE         float64         `json:"occupied_fte"`
		StaffingState       string          `json:"staffing_state"`
		PositionType        *string         `json:"position_type,omitempty"`
		EmploymentType      *string         `json:"employment_type,omitempty"`
		ReportsToPositionID *uuid.UUID      `json:"reports_to_position_id,omitempty"`
		JobFamilyGroupCode  *string         `json:"job_family_group_code,omitempty"`
		JobFamilyCode       *string         `json:"job_family_code,omitempty"`
		JobRoleCode         *string         `json:"job_role_code,omitempty"`
		JobLevelCode        *string         `json:"job_level_code,omitempty"`
		JobProfileID        *uuid.UUID      `json:"job_profile_id,omitempty"`
		CostCenterCode      *string         `json:"cost_center_code,omitempty"`
		Profile             json.RawMessage `json:"profile,omitempty"`
		EffectiveDate       string          `json:"effective_date"`
		EndDate             string          `json:"end_date"`
	}

	positions := make([]positionViewRowResponse, 0, len(rows))
	for _, row := range rows {
		positions = append(positions, positionViewRowResponse{
			PositionID:          row.PositionID,
			Code:                row.Code,
			OrgNodeID:           row.OrgNodeID,
			Title:               row.Title,
			LifecycleStatus:     row.LifecycleStatus,
			IsAutoCreated:       row.IsAutoCreated,
			CapacityFTE:         row.CapacityFTE,
			OccupiedFTE:         row.OccupiedFTE,
			StaffingState:       row.StaffingState,
			PositionType:        row.PositionType,
			EmploymentType:      row.EmploymentType,
			ReportsToPositionID: row.ReportsToPositionID,
			JobFamilyGroupCode:  row.JobFamilyGroupCode,
			JobFamilyCode:       row.JobFamilyCode,
			JobRoleCode:         row.JobRoleCode,
			JobLevelCode:        row.JobLevelCode,
			JobProfileID:        row.JobProfileID,
			CostCenterCode:      row.CostCenterCode,
			Profile:             row.Profile,
			EffectiveDate:       formatValidDate(row.EffectiveDate),
			EndDate:             formatValidEndDateFromEndDate(row.EndDate),
		})
	}

	type getPositionsResponse struct {
		TenantID  string                    `json:"tenant_id"`
		AsOf      string                    `json:"as_of"`
		Page      int                       `json:"page"`
		Limit     int                       `json:"limit"`
		Positions []positionViewRowResponse `json:"positions"`
	}
	writeJSON(w, http.StatusOK, getPositionsResponse{
		TenantID:  tenantID.String(),
		AsOf:      formatValidDate(effectiveDate),
		Page:      page,
		Limit:     limit,
		Positions: positions,
	})
}

type createPositionRequest struct {
	Code               string          `json:"code"`
	OrgNodeID          uuid.UUID       `json:"org_node_id"`
	EffectiveDate      string          `json:"effective_date"`
	Title              *string         `json:"title"`
	LifecycleStatus    string          `json:"lifecycle_status"`
	PositionType       string          `json:"position_type"`
	EmploymentType     string          `json:"employment_type"`
	CapacityFTE        *float64        `json:"capacity_fte"`
	ReportsToID        *uuid.UUID      `json:"reports_to_position_id"`
	JobFamilyGroupCode string          `json:"job_family_group_code"`
	JobFamilyCode      string          `json:"job_family_code"`
	JobRoleCode        string          `json:"job_role_code"`
	JobLevelCode       string          `json:"job_level_code"`
	JobProfileID       *uuid.UUID      `json:"job_profile_id"`
	CostCenterCode     *string         `json:"cost_center_code"`
	Profile            json.RawMessage `json:"profile"`
	ReasonCode         string          `json:"reason_code"`
	ReasonNote         *string         `json:"reason_note"`
}

func (c *OrgAPIController) CreatePosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}

	var req createPositionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	effectiveDate, err := parseRequiredEffectiveDate(req.EffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "effective_date is required")
		return
	}
	capacityFTE := 1.0
	if req.CapacityFTE != nil {
		capacityFTE = *req.CapacityFTE
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.CreatePosition(r.Context(), tenantID, requestID, initiatorID, services.CreatePositionInput{
		Code:               req.Code,
		OrgNodeID:          req.OrgNodeID,
		EffectiveDate:      effectiveDate,
		Title:              req.Title,
		LifecycleStatus:    req.LifecycleStatus,
		PositionType:       req.PositionType,
		EmploymentType:     req.EmploymentType,
		CapacityFTE:        capacityFTE,
		ReportsToID:        req.ReportsToID,
		JobFamilyGroupCode: req.JobFamilyGroupCode,
		JobFamilyCode:      req.JobFamilyCode,
		JobRoleCode:        req.JobRoleCode,
		JobLevelCode:       req.JobLevelCode,
		JobProfileID:       req.JobProfileID,
		CostCenterCode:     req.CostCenterCode,
		Profile:            req.Profile,
		ReasonCode:         req.ReasonCode,
		ReasonNote:         req.ReasonNote,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type createPositionResponse struct {
		PositionID      string                  `json:"position_id"`
		SliceID         string                  `json:"slice_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusCreated, createPositionResponse{
		PositionID: res.PositionID.String(),
		SliceID:    res.SliceID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

func (c *OrgAPIController) GetPosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
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

	row, effectiveDate, err := c.org.GetPosition(r.Context(), tenantID, positionID, asOf)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type positionViewRowResponse struct {
		PositionID          uuid.UUID       `json:"position_id"`
		Code                string          `json:"code"`
		OrgNodeID           uuid.UUID       `json:"org_node_id"`
		Title               *string         `json:"title,omitempty"`
		LifecycleStatus     string          `json:"lifecycle_status"`
		IsAutoCreated       bool            `json:"is_auto_created"`
		CapacityFTE         float64         `json:"capacity_fte"`
		OccupiedFTE         float64         `json:"occupied_fte"`
		StaffingState       string          `json:"staffing_state"`
		PositionType        *string         `json:"position_type,omitempty"`
		EmploymentType      *string         `json:"employment_type,omitempty"`
		ReportsToPositionID *uuid.UUID      `json:"reports_to_position_id,omitempty"`
		JobFamilyGroupCode  *string         `json:"job_family_group_code,omitempty"`
		JobFamilyCode       *string         `json:"job_family_code,omitempty"`
		JobRoleCode         *string         `json:"job_role_code,omitempty"`
		JobLevelCode        *string         `json:"job_level_code,omitempty"`
		JobProfileID        *uuid.UUID      `json:"job_profile_id,omitempty"`
		CostCenterCode      *string         `json:"cost_center_code,omitempty"`
		Profile             json.RawMessage `json:"profile,omitempty"`
		EffectiveDate       string          `json:"effective_date"`
		EndDate             string          `json:"end_date"`
	}

	type getPositionResponse struct {
		TenantID string                  `json:"tenant_id"`
		AsOf     string                  `json:"as_of"`
		Position positionViewRowResponse `json:"position"`
	}
	writeJSON(w, http.StatusOK, getPositionResponse{
		TenantID: tenantID.String(),
		AsOf:     formatValidDate(effectiveDate),
		Position: positionViewRowResponse{
			PositionID:          row.PositionID,
			Code:                row.Code,
			OrgNodeID:           row.OrgNodeID,
			Title:               row.Title,
			LifecycleStatus:     row.LifecycleStatus,
			IsAutoCreated:       row.IsAutoCreated,
			CapacityFTE:         row.CapacityFTE,
			OccupiedFTE:         row.OccupiedFTE,
			StaffingState:       row.StaffingState,
			PositionType:        row.PositionType,
			EmploymentType:      row.EmploymentType,
			ReportsToPositionID: row.ReportsToPositionID,
			JobFamilyGroupCode:  row.JobFamilyGroupCode,
			JobFamilyCode:       row.JobFamilyCode,
			JobRoleCode:         row.JobRoleCode,
			JobLevelCode:        row.JobLevelCode,
			JobProfileID:        row.JobProfileID,
			CostCenterCode:      row.CostCenterCode,
			Profile:             row.Profile,
			EffectiveDate:       formatValidDate(row.EffectiveDate),
			EndDate:             formatValidEndDateFromEndDate(row.EndDate),
		},
	})
}

func (c *OrgAPIController) GetPositionTimeline(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	rows, err := c.org.GetPositionTimeline(r.Context(), tenantID, positionID)
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type getPositionTimelineResponse struct {
		TenantID   string                      `json:"tenant_id"`
		PositionID string                      `json:"position_id"`
		Slices     []services.PositionSliceRow `json:"slices"`
	}
	writeJSON(w, http.StatusOK, getPositionTimelineResponse{
		TenantID:   tenantID.String(),
		PositionID: positionID.String(),
		Slices:     rows,
	})
}

type updatePositionRequest struct {
	EffectiveDate      string          `json:"effective_date"`
	ReasonCode         string          `json:"reason_code"`
	ReasonNote         *string         `json:"reason_note"`
	OrgNodeID          *uuid.UUID      `json:"org_node_id"`
	Title              *string         `json:"title"`
	LifecycleStatus    *string         `json:"lifecycle_status"`
	PositionType       *string         `json:"position_type"`
	EmploymentType     *string         `json:"employment_type"`
	CapacityFTE        *float64        `json:"capacity_fte"`
	ReportsToID        *uuid.UUID      `json:"reports_to_position_id"`
	JobFamilyGroupCode *string         `json:"job_family_group_code"`
	JobFamilyCode      *string         `json:"job_family_code"`
	JobRoleCode        *string         `json:"job_role_code"`
	JobLevelCode       *string         `json:"job_level_code"`
	JobProfileID       *uuid.UUID      `json:"job_profile_id"`
	CostCenterCode     *string         `json:"cost_center_code"`
	Profile            json.RawMessage `json:"profile"`
}

func (c *OrgAPIController) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req updatePositionRequest
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
	var profile *json.RawMessage
	if req.Profile != nil {
		tmp := req.Profile
		profile = &tmp
	}
	res, err := c.org.UpdatePosition(r.Context(), tenantID, requestID, initiatorID, services.UpdatePositionInput{
		PositionID:         positionID,
		EffectiveDate:      effectiveDate,
		ReasonCode:         req.ReasonCode,
		ReasonNote:         req.ReasonNote,
		OrgNodeID:          req.OrgNodeID,
		Title:              req.Title,
		LifecycleStatus:    req.LifecycleStatus,
		PositionType:       req.PositionType,
		EmploymentType:     req.EmploymentType,
		CapacityFTE:        req.CapacityFTE,
		ReportsToID:        req.ReportsToID,
		JobFamilyGroupCode: req.JobFamilyGroupCode,
		JobFamilyCode:      req.JobFamilyCode,
		JobRoleCode:        req.JobRoleCode,
		JobLevelCode:       req.JobLevelCode,
		JobProfileID:       req.JobProfileID,
		CostCenterCode:     req.CostCenterCode,
		Profile:            profile,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type updatePositionResponse struct {
		PositionID      string                  `json:"position_id"`
		SliceID         string                  `json:"slice_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, updatePositionResponse{
		PositionID: res.PositionID.String(),
		SliceID:    res.SliceID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type correctPositionRequest struct {
	EffectiveDate      string          `json:"effective_date"`
	ReasonCode         string          `json:"reason_code"`
	ReasonNote         *string         `json:"reason_note"`
	OrgNodeID          *uuid.UUID      `json:"org_node_id"`
	Title              *string         `json:"title"`
	LifecycleStatus    *string         `json:"lifecycle_status"`
	PositionType       *string         `json:"position_type"`
	EmploymentType     *string         `json:"employment_type"`
	CapacityFTE        *float64        `json:"capacity_fte"`
	ReportsToID        *uuid.UUID      `json:"reports_to_position_id"`
	JobFamilyGroupCode *string         `json:"job_family_group_code"`
	JobFamilyCode      *string         `json:"job_family_code"`
	JobRoleCode        *string         `json:"job_role_code"`
	JobLevelCode       *string         `json:"job_level_code"`
	JobProfileID       *uuid.UUID      `json:"job_profile_id"`
	CostCenterCode     *string         `json:"cost_center_code"`
	Profile            json.RawMessage `json:"profile"`
}

func (c *OrgAPIController) CorrectPosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "admin") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req correctPositionRequest
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
	var profile *json.RawMessage
	if req.Profile != nil {
		tmp := req.Profile
		profile = &tmp
	}
	res, err := c.org.CorrectPosition(r.Context(), tenantID, requestID, initiatorID, services.CorrectPositionInput{
		PositionID:         positionID,
		AsOf:               effectiveDate,
		ReasonCode:         req.ReasonCode,
		ReasonNote:         req.ReasonNote,
		OrgNodeID:          req.OrgNodeID,
		Title:              req.Title,
		Lifecycle:          req.LifecycleStatus,
		PositionType:       req.PositionType,
		EmploymentType:     req.EmploymentType,
		CapacityFTE:        req.CapacityFTE,
		ReportsToID:        req.ReportsToID,
		JobFamilyGroupCode: req.JobFamilyGroupCode,
		JobFamilyCode:      req.JobFamilyCode,
		JobRoleCode:        req.JobRoleCode,
		JobLevelCode:       req.JobLevelCode,
		JobProfileID:       req.JobProfileID,
		CostCenterCode:     req.CostCenterCode,
		Profile:            profile,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type correctPositionResponse struct {
		PositionID      string                  `json:"position_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, correctPositionResponse{
		PositionID: res.PositionID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type rescindPositionRequest struct {
	EffectiveDate string  `json:"effective_date"`
	ReasonCode    string  `json:"reason_code"`
	ReasonNote    *string `json:"reason_note"`
}

func (c *OrgAPIController) RescindPosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "admin") {
		return
	}

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}

	var req rescindPositionRequest
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
	res, err := c.org.RescindPosition(r.Context(), tenantID, requestID, initiatorID, services.RescindPositionInput{
		PositionID:    positionID,
		EffectiveDate: effectiveDate,
		ReasonCode:    req.ReasonCode,
		ReasonNote:    req.ReasonNote,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type rescindPositionResponse struct {
		PositionID      string                  `json:"position_id"`
		EffectiveWindow effectiveWindowResponse `json:"effective_window"`
	}
	writeJSON(w, http.StatusOK, rescindPositionResponse{
		PositionID: res.PositionID.String(),
		EffectiveWindow: effectiveWindowResponse{
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type shiftBoundaryPositionRequest struct {
	TargetEffectiveDate string  `json:"target_effective_date"`
	NewEffectiveDate    string  `json:"new_effective_date"`
	ReasonCode          string  `json:"reason_code"`
	ReasonNote          *string `json:"reason_note"`
}

func (c *OrgAPIController) ShiftBoundaryPosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgPositionsAuthzObject, "admin") {
		return
	}
	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_QUERY", "invalid id")
		return
	}
	var req shiftBoundaryPositionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "invalid json body")
		return
	}
	target, err := parseRequiredValidDate("target_effective_date", req.TargetEffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "target_effective_date is required")
		return
	}
	newStart, err := parseRequiredValidDate("new_effective_date", req.NewEffectiveDate)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, requestID, "ORG_INVALID_BODY", "new_effective_date is required")
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	res, err := c.org.ShiftBoundaryPosition(r.Context(), tenantID, requestID, initiatorID, services.ShiftBoundaryPositionInput{
		PositionID:          positionID,
		TargetEffectiveDate: target,
		NewEffectiveDate:    newStart,
		ReasonCode:          req.ReasonCode,
		ReasonNote:          req.ReasonNote,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	type shiftBoundaryPositionResponse struct {
		PositionID string `json:"position_id"`
		Shifted    struct {
			TargetEffectiveDate string `json:"target_effective_date"`
			NewEffectiveDate    string `json:"new_effective_date"`
		} `json:"shifted"`
	}
	var resp shiftBoundaryPositionResponse
	resp.PositionID = res.PositionID.String()
	resp.Shifted.TargetEffectiveDate = formatValidDate(res.TargetStart)
	resp.Shifted.NewEffectiveDate = formatValidDate(res.NewStart)
	writeJSON(w, http.StatusOK, resp)
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

	type assignmentViewRowResponse struct {
		ID             uuid.UUID  `json:"id"`
		PositionID     uuid.UUID  `json:"position_id"`
		OrgNodeID      uuid.UUID  `json:"org_node_id"`
		AssignmentType string     `json:"assignment_type"`
		IsPrimary      bool       `json:"is_primary"`
		AllocatedFTE   float64    `json:"allocated_fte"`
		EffectiveDate  string     `json:"effective_date"`
		EndDate        string     `json:"end_date"`
		PositionCode   *string    `json:"position_code,omitempty"`
		Pernr          *string    `json:"pernr,omitempty"`
		SubjectID      *uuid.UUID `json:"subject_id,omitempty"`
		StartEventType *string    `json:"start_event_type,omitempty"`
		EndEventType   *string    `json:"end_event_type,omitempty"`
	}

	assignments := make([]assignmentViewRowResponse, 0, len(rows))
	for _, row := range rows {
		assignments = append(assignments, assignmentViewRowResponse{
			ID:             row.ID,
			PositionID:     row.PositionID,
			OrgNodeID:      row.OrgNodeID,
			AssignmentType: row.AssignmentType,
			IsPrimary:      row.IsPrimary,
			AllocatedFTE:   row.AllocatedFTE,
			EffectiveDate:  formatValidDate(row.EffectiveDate),
			EndDate:        formatValidEndDateFromEndDate(row.EndDate),
			PositionCode:   row.PositionCode,
			Pernr:          row.Pernr,
			SubjectID:      row.SubjectID,
			StartEventType: row.StartEventType,
			EndEventType:   row.EndEventType,
		})
	}

	type getAssignmentsResponse struct {
		TenantID      string                      `json:"tenant_id"`
		Subject       string                      `json:"subject"`
		SubjectID     string                      `json:"subject_id"`
		EffectiveDate *string                     `json:"effective_date,omitempty"`
		Assignments   []assignmentViewRowResponse `json:"assignments"`
	}
	var effectiveDateOut *string
	if !effectiveDate.IsZero() {
		v := formatValidDate(effectiveDate)
		effectiveDateOut = &v
	}
	writeJSON(w, http.StatusOK, getAssignmentsResponse{
		TenantID:      tenantID.String(),
		Subject:       subject,
		SubjectID:     subjectID.String(),
		EffectiveDate: effectiveDateOut,
		Assignments:   assignments,
	})
}

type createAssignmentRequest struct {
	Pernr          string     `json:"pernr"`
	EffectiveDate  string     `json:"effective_date"`
	ReasonCode     string     `json:"reason_code"`
	ReasonNote     *string    `json:"reason_note"`
	AssignmentType string     `json:"assignment_type"`
	AllocatedFTE   *float64   `json:"allocated_fte"`
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
		ReasonCode:     req.ReasonCode,
		ReasonNote:     req.ReasonNote,
		AssignmentType: req.AssignmentType,
		AllocatedFTE: func() float64 {
			if req.AllocatedFTE == nil {
				return 0
			}
			return *req.AllocatedFTE
		}(),
		PositionID: req.PositionID,
		OrgNodeID:  req.OrgNodeID,
		SubjectID:  req.SubjectID,
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type hirePersonnelEventRequest struct {
	Pernr         string     `json:"pernr"`
	OrgNodeID     uuid.UUID  `json:"org_node_id"`
	PositionID    *uuid.UUID `json:"position_id"`
	EffectiveDate string     `json:"effective_date"`
	AllocatedFTE  *float64   `json:"allocated_fte"`
	ReasonCode    string     `json:"reason_code"`
}

type transferPersonnelEventRequest struct {
	Pernr         string     `json:"pernr"`
	OrgNodeID     uuid.UUID  `json:"org_node_id"`
	PositionID    *uuid.UUID `json:"position_id"`
	EffectiveDate string     `json:"effective_date"`
	AllocatedFTE  *float64   `json:"allocated_fte"`
	ReasonCode    string     `json:"reason_code"`
}

type terminationPersonnelEventRequest struct {
	Pernr         string `json:"pernr"`
	EffectiveDate string `json:"effective_date"`
	ReasonCode    string `json:"reason_code"`
}

type personnelEventResponse struct {
	PersonnelEventID string `json:"personnel_event_id"`
	EventType        string `json:"event_type"`
	PersonUUID       string `json:"person_uuid"`
	Pernr            string `json:"pernr"`
	EffectiveDate    string `json:"effective_date"`
	ReasonCode       string `json:"reason_code"`
}

func (c *OrgAPIController) HirePersonnelEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	var req hirePersonnelEventRequest
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
	res, err := c.org.HirePersonnelEvent(r.Context(), tenantID, requestID, initiatorID, services.HirePersonnelEventInput{
		Pernr:         req.Pernr,
		OrgNodeID:     req.OrgNodeID,
		PositionID:    req.PositionID,
		EffectiveDate: effectiveDate,
		AllocatedFTE: func() float64 {
			if req.AllocatedFTE == nil {
				return 0
			}
			return *req.AllocatedFTE
		}(),
		ReasonCode: req.ReasonCode,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}

	writeJSON(w, status, personnelEventResponse{
		PersonnelEventID: res.Event.ID.String(),
		EventType:        res.Event.EventType,
		PersonUUID:       res.Event.PersonUUID.String(),
		Pernr:            res.Event.Pernr,
		EffectiveDate:    formatValidDate(res.Event.EffectiveDate),
		ReasonCode:       res.Event.ReasonCode,
	})
}

func (c *OrgAPIController) TransferPersonnelEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	var req transferPersonnelEventRequest
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
	res, err := c.org.TransferPersonnelEvent(r.Context(), tenantID, requestID, initiatorID, services.TransferPersonnelEventInput{
		Pernr:         req.Pernr,
		OrgNodeID:     req.OrgNodeID,
		PositionID:    req.PositionID,
		EffectiveDate: effectiveDate,
		AllocatedFTE: func() float64 {
			if req.AllocatedFTE == nil {
				return 0
			}
			return *req.AllocatedFTE
		}(),
		ReasonCode: req.ReasonCode,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}

	writeJSON(w, status, personnelEventResponse{
		PersonnelEventID: res.Event.ID.String(),
		EventType:        res.Event.EventType,
		PersonUUID:       res.Event.PersonUUID.String(),
		Pernr:            res.Event.Pernr,
		EffectiveDate:    formatValidDate(res.Event.EffectiveDate),
		ReasonCode:       res.Event.ReasonCode,
	})
}

func (c *OrgAPIController) TerminationPersonnelEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, requestID, ok := requireSessionTenantUser(w, r)
	if !ok {
		return
	}
	if !ensureOrgAuthz(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	var req terminationPersonnelEventRequest
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
	res, err := c.org.TerminationPersonnelEvent(r.Context(), tenantID, requestID, initiatorID, services.TerminationPersonnelEventInput{
		Pernr:         req.Pernr,
		EffectiveDate: effectiveDate,
		ReasonCode:    req.ReasonCode,
	})
	if err != nil {
		writeServiceError(w, requestID, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}

	writeJSON(w, status, personnelEventResponse{
		PersonnelEventID: res.Event.ID.String(),
		EventType:        res.Event.EventType,
		PersonUUID:       res.Event.PersonUUID.String(),
		Pernr:            res.Event.Pernr,
		EffectiveDate:    formatValidDate(res.Event.EffectiveDate),
		ReasonCode:       res.Event.ReasonCode,
	})
}

type updateAssignmentRequest struct {
	EffectiveDate string       `json:"effective_date"`
	ReasonCode    string       `json:"reason_code"`
	ReasonNote    *string      `json:"reason_note"`
	EndDate       *string      `json:"end_date"`
	PositionID    optionalUUID `json:"position_id"`
	OrgNodeID     optionalUUID `json:"org_node_id"`
	AllocatedFTE  *float64     `json:"allocated_fte"`
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
		ReasonCode:    req.ReasonCode,
		ReasonNote:    req.ReasonNote,
		AllocatedFTE:  req.AllocatedFTE,
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type correctAssignmentRequest struct {
	ReasonCode string     `json:"reason_code"`
	ReasonNote *string    `json:"reason_note"`
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
		ReasonCode:   req.ReasonCode,
		ReasonNote:   req.ReasonNote,
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
		},
	})
}

type rescindAssignmentRequest struct {
	EffectiveDate string  `json:"effective_date"`
	ReasonCode    string  `json:"reason_code"`
	ReasonNote    *string `json:"reason_note"`
	Reason        string  `json:"reason"`
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
		ReasonCode:    req.ReasonCode,
		ReasonNote:    req.ReasonNote,
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
			EffectiveDate: formatValidDate(res.EffectiveDate),
			EndDate:       formatValidEndDateFromEndDate(res.EndDate),
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
	type snapshotResponse struct {
		TenantID      uuid.UUID               `json:"tenant_id"`
		EffectiveDate string                  `json:"effective_date"`
		GeneratedAt   string                  `json:"generated_at"`
		Includes      []string                `json:"includes"`
		Limit         int                     `json:"limit"`
		Items         []services.SnapshotItem `json:"items"`
		NextCursor    *string                 `json:"next_cursor"`
	}
	writeJSON(w, http.StatusOK, snapshotResponse{
		TenantID:      res.TenantID,
		EffectiveDate: formatValidDate(res.EffectiveDate),
		GeneratedAt:   res.GeneratedAt.UTC().Format(time.RFC3339),
		Includes:      res.Includes,
		Limit:         res.Limit,
		Items:         res.Items,
		NextCursor:    res.NextCursor,
	})
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
		globalEffective = formatValidDate(time.Now())
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
			target, err := parseRequiredValidDate("target_effective_date", body.TargetEffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "target_effective_date is required")
				return
			}
			newStart, err := parseRequiredValidDate("new_effective_date", body.NewEffectiveDate)
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

		case "security_group_mapping.create":
			if !configuration.Use().OrgSecurityGroupMappingsEnabled {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusNotFound, "ORG_NOT_FOUND", "not found")
				return
			}
			var body createSecurityGroupMappingRequest
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CreateSecurityGroupMapping(txCtx, tenantID, requestID, initiatorID, services.CreateSecurityGroupMappingInput{
				OrgNodeID:        body.OrgNodeID,
				SecurityGroupKey: body.SecurityGroupKey,
				AppliesToSubtree: body.AppliesToSubtree,
				EffectiveDate:    effectiveDate,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.ID.String()}})

		case "security_group_mapping.rescind":
			if !configuration.Use().OrgSecurityGroupMappingsEnabled {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusNotFound, "ORG_NOT_FOUND", "not found")
				return
			}
			var body struct {
				ID uuid.UUID `json:"id"`
				rescindSecurityGroupMappingRequest
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
			res, err := c.org.RescindSecurityGroupMapping(txCtx, tenantID, requestID, initiatorID, services.RescindSecurityGroupMappingInput{
				ID:            body.ID,
				EffectiveDate: effectiveDate,
				Reason:        body.Reason,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.ID.String()}})

		case "link.create":
			if !configuration.Use().OrgLinksEnabled {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusNotFound, "ORG_NOT_FOUND", "not found")
				return
			}
			var body createLinkRequest
			if err := json.Unmarshal(payload, &body); err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "payload is invalid")
				return
			}
			effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
			if err != nil {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusUnprocessableEntity, "ORG_BATCH_INVALID_COMMAND", "effective_date is required")
				return
			}
			res, err := c.org.CreateOrgLink(txCtx, tenantID, requestID, initiatorID, services.CreateOrgLinkInput{
				OrgNodeID:     body.OrgNodeID,
				ObjectType:    body.ObjectType,
				ObjectKey:     body.ObjectKey,
				LinkType:      body.LinkType,
				Metadata:      body.Metadata,
				EffectiveDate: effectiveDate,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.ID.String()}})

		case "link.rescind":
			if !configuration.Use().OrgLinksEnabled {
				writeBatchCommandError(w, requestID, i, cmdType, http.StatusNotFound, "ORG_NOT_FOUND", "not found")
				return
			}
			var body struct {
				ID uuid.UUID `json:"id"`
				rescindLinkRequest
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
			res, err := c.org.RescindOrgLink(txCtx, tenantID, requestID, initiatorID, services.RescindOrgLinkInput{
				ID:            body.ID,
				EffectiveDate: effectiveDate,
				Reason:        body.Reason,
			})
			if err != nil {
				writeBatchServiceError(w, requestID, i, cmdType, err)
				return
			}
			results = append(results, batchCommandResult{Index: i, Type: cmdType, Ok: true, Result: map[string]any{"id": res.ID.String()}})

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
	c.org.InvalidateTenantCacheWithReason(tenantID, "write_commit")

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
		globalEffective = formatValidDate(time.Now())
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
		EffectiveDate: formatValidDate(asOf),
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
		targetDate, err := parseRequiredValidDate("target_effective_date", body.TargetEffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "target_effective_date is required", Cause: err}
		}
		newDate, err := parseRequiredValidDate("new_effective_date", body.NewEffectiveDate)
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
	case "security_group_mapping.create":
		if !configuration.Use().OrgSecurityGroupMappingsEnabled {
			return &services.ServiceError{Status: http.StatusNotFound, Code: "ORG_NOT_FOUND", Message: "not found"}
		}
		var body createSecurityGroupMappingRequest
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.CreateSecurityGroupMapping(ctx, tenantID, requestID, initiatorID, services.CreateSecurityGroupMappingInput{
			OrgNodeID:        body.OrgNodeID,
			SecurityGroupKey: body.SecurityGroupKey,
			AppliesToSubtree: body.AppliesToSubtree,
			EffectiveDate:    effectiveDate,
		})
		return err
	case "security_group_mapping.rescind":
		if !configuration.Use().OrgSecurityGroupMappingsEnabled {
			return &services.ServiceError{Status: http.StatusNotFound, Code: "ORG_NOT_FOUND", Message: "not found"}
		}
		var body struct {
			ID uuid.UUID `json:"id"`
			rescindSecurityGroupMappingRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.RescindSecurityGroupMapping(ctx, tenantID, requestID, initiatorID, services.RescindSecurityGroupMappingInput{
			ID:            body.ID,
			EffectiveDate: effectiveDate,
			Reason:        body.Reason,
		})
		return err
	case "link.create":
		if !configuration.Use().OrgLinksEnabled {
			return &services.ServiceError{Status: http.StatusNotFound, Code: "ORG_NOT_FOUND", Message: "not found"}
		}
		var body createLinkRequest
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.CreateOrgLink(ctx, tenantID, requestID, initiatorID, services.CreateOrgLinkInput{
			OrgNodeID:     body.OrgNodeID,
			ObjectType:    body.ObjectType,
			ObjectKey:     body.ObjectKey,
			LinkType:      body.LinkType,
			Metadata:      body.Metadata,
			EffectiveDate: effectiveDate,
		})
		return err
	case "link.rescind":
		if !configuration.Use().OrgLinksEnabled {
			return &services.ServiceError{Status: http.StatusNotFound, Code: "ORG_NOT_FOUND", Message: "not found"}
		}
		var body struct {
			ID uuid.UUID `json:"id"`
			rescindLinkRequest
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "payload is invalid", Cause: err}
		}
		effectiveDate, err := parseRequiredEffectiveDate(body.EffectiveDate)
		if err != nil {
			return &services.ServiceError{Status: http.StatusUnprocessableEntity, Code: "ORG_PREFLIGHT_INVALID_COMMAND", Message: "effective_date is required", Cause: err}
		}
		_, err = c.org.RescindOrgLink(ctx, tenantID, requestID, initiatorID, services.RescindOrgLinkInput{
			ID:            body.ID,
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
	case "security_group_mapping.create", "security_group_mapping.rescind":
		return true
	case "link.create", "link.rescind":
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

func requireOrgSecurityGroupMappingsEnabled(w http.ResponseWriter, requestID string) bool {
	if configuration.Use().OrgSecurityGroupMappingsEnabled {
		return true
	}
	writeAPIError(w, http.StatusNotFound, requestID, "ORG_NOT_FOUND", "not found")
	return false
}

func requireOrgLinksEnabled(w http.ResponseWriter, requestID string) bool {
	if configuration.Use().OrgLinksEnabled {
		return true
	}
	writeAPIError(w, http.StatusNotFound, requestID, "ORG_NOT_FOUND", "not found")
	return false
}

func requireOrgPermissionPreviewEnabled(w http.ResponseWriter, requestID string) bool {
	if configuration.Use().OrgPermissionPreviewEnabled {
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

func parseEffectiveIDCursor(raw string) (*time.Time, *uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}
	if !strings.HasPrefix(raw, "effective_date:") {
		return nil, nil, fmt.Errorf("invalid cursor")
	}
	rest := strings.TrimPrefix(raw, "effective_date:")
	atStr, idStr, ok := strings.Cut(rest, ":id:")
	if !ok || strings.TrimSpace(atStr) == "" || strings.TrimSpace(idStr) == "" {
		return nil, nil, fmt.Errorf("invalid cursor")
	}

	at, err := parseEffectiveDate(atStr)
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

func parseIDCursor(raw string) (uuid.UUID, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, false, nil
	}
	if !strings.HasPrefix(raw, "id:") {
		return uuid.Nil, false, fmt.Errorf("invalid cursor")
	}
	idStr := strings.TrimSpace(strings.TrimPrefix(raw, "id:"))
	if idStr == "" {
		return uuid.Nil, false, fmt.Errorf("invalid cursor")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
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

func parseOptionalValidDate(field, v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be YYYY-MM-DD", field)
	}
	return normalizeValidTimeDayUTC(t), nil
}

func parseRequiredValidDate(field, v string) (time.Time, error) {
	t, err := parseOptionalValidDate(field, v)
	if err != nil {
		return time.Time{}, err
	}
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	return t, nil
}

func parseEffectiveDate(v string) (time.Time, error) {
	return parseOptionalValidDate("effective_date", v)
}

func parseRequiredEffectiveDate(v string) (time.Time, error) {
	return parseRequiredValidDate("effective_date", v)
}

func normalizeValidTimeDayUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func formatValidDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return normalizeValidTimeDayUTC(t).Format(time.DateOnly)
}

func formatValidEndDateFromEndDate(endDate time.Time) string {
	if endDate.IsZero() {
		return ""
	}
	u := endDate.UTC()
	y, m, d := u.Date()
	if y == 9999 && m == time.December && d == 31 {
		return "9999-12-31"
	}
	return normalizeValidTimeDayUTC(u).Format(time.DateOnly)
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
