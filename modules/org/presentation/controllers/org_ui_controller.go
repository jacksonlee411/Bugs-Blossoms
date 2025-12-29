package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/components/base"
	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/layouts"
	"github.com/iota-uz/iota-sdk/modules/org/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/org/presentation/templates/components/orgui"
	orgtemplates "github.com/iota-uz/iota-sdk/modules/org/presentation/templates/pages/org"
	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
	"github.com/iota-uz/iota-sdk/pkg/orglabels"
)

type OrgUIController struct {
	app      application.Application
	org      *services.OrgService
	basePath string
}

func NewOrgUIController(app application.Application) application.Controller {
	return &OrgUIController{
		app:      app,
		org:      app.Service(services.OrgService{}).(*services.OrgService),
		basePath: "/org",
	}
}

func (c *OrgUIController) Key() string {
	return c.basePath
}

func (c *OrgUIController) Register(r *mux.Router) {
	common := []mux.MiddlewareFunc{
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	}

	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(common...)
	router.HandleFunc("", c.RedirectRoot).Methods(http.MethodGet)

	router.HandleFunc("/nodes", c.NodesPage).Methods(http.MethodGet)
	router.HandleFunc("/assignments", c.AssignmentsPage).Methods(http.MethodGet)
	router.HandleFunc("/hierarchies", c.HierarchyPartial).Methods(http.MethodGet)
	router.HandleFunc("/nodes/search", c.NodeSearchOptions).Methods(http.MethodGet)
	router.HandleFunc("/nodes/new", c.NewNodeForm).Methods(http.MethodGet)
	router.HandleFunc("/nodes", c.CreateNode).Methods(http.MethodPost)
	router.HandleFunc("/nodes/{id}", c.NodePanel).Methods(http.MethodGet)
	router.HandleFunc("/nodes/{id}", c.UpdateNode).Methods(http.MethodPatch)
	router.HandleFunc("/nodes/{id}:move", c.MoveNode).Methods(http.MethodPost)

	router.HandleFunc("/assignments/form", c.AssignmentForm).Methods(http.MethodGet)
	router.HandleFunc("/assignments", c.CreateAssignment).Methods(http.MethodPost)
	router.HandleFunc("/assignments/{id}/transition", c.TransitionAssignmentForm).Methods(http.MethodGet)
	router.HandleFunc("/assignments/{id}:transition", c.TransitionAssignment).Methods(http.MethodPost)
	router.HandleFunc("/assignments/{id}/edit", c.EditAssignmentForm).Methods(http.MethodGet)
	router.HandleFunc("/assignments/{id}", c.UpdateAssignment).Methods(http.MethodPatch)

	router.HandleFunc("/positions", c.PositionsPage).Methods(http.MethodGet)
	router.HandleFunc("/positions/panel", c.PositionsPanel).Methods(http.MethodGet)
	router.HandleFunc("/positions/search", c.PositionSearchOptions).Methods(http.MethodGet)
	router.HandleFunc("/positions/new", c.NewPositionForm).Methods(http.MethodGet)
	router.HandleFunc("/positions", c.CreatePosition).Methods(http.MethodPost)
	router.HandleFunc("/positions/{id}", c.PositionDetails).Methods(http.MethodGet)
	router.HandleFunc("/positions/{id}/edit", c.EditPositionForm).Methods(http.MethodGet)
	router.HandleFunc("/positions/{id}", c.UpdatePosition).Methods(http.MethodPatch)

	router.HandleFunc("/job-catalog", c.JobCatalogPage).Methods(http.MethodGet)
	router.HandleFunc("/job-catalog/family-groups", c.CreateJobFamilyGroupUI).Methods(http.MethodPost)
	router.HandleFunc("/job-catalog/family-groups/{id}", c.UpdateJobFamilyGroupUI).Methods(http.MethodPatch)
	router.HandleFunc("/job-catalog/families", c.CreateJobFamilyUI).Methods(http.MethodPost)
	router.HandleFunc("/job-catalog/families/{id}", c.UpdateJobFamilyUI).Methods(http.MethodPatch)
	router.HandleFunc("/job-catalog/roles", c.CreateJobRoleUI).Methods(http.MethodPost)
	router.HandleFunc("/job-catalog/roles/{id}", c.UpdateJobRoleUI).Methods(http.MethodPatch)
	router.HandleFunc("/job-catalog/levels", c.CreateJobLevelUI).Methods(http.MethodPost)
	router.HandleFunc("/job-catalog/levels/{id}", c.UpdateJobLevelUI).Methods(http.MethodPatch)

	router.HandleFunc("/job-catalog/family-groups/options", c.JobFamilyGroupOptions).Methods(http.MethodGet)
	router.HandleFunc("/job-catalog/families/options", c.JobFamilyOptions).Methods(http.MethodGet)
	router.HandleFunc("/job-catalog/roles/options", c.JobRoleOptions).Methods(http.MethodGet)
	router.HandleFunc("/job-catalog/levels/options", c.JobLevelOptions).Methods(http.MethodGet)
}

func (c *OrgUIController) RedirectRoot(w http.ResponseWriter, r *http.Request) {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil || !services.OrgRolloutEnabledForTenant(tenantID) {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/org/nodes", http.StatusFound)
}

func ensureOrgRolloutEnabled(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) bool {
	if !services.OrgRolloutEnabledForTenant(tenantID) {
		http.NotFound(w, r)
		return false
	}
	return true
}

func (c *OrgUIController) NodesPage(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgHierarchiesAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgNodesAuthzObject, "write")
	ensureOrgPageCapabilities(r, orgEdgesAuthzObject, "write")

	statusCode := http.StatusOK
	var errs []string
	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil {
		statusCode = http.StatusBadRequest
		errs = append(errs, "invalid effective_date")
		effectiveDate = time.Now().UTC()
	}
	if effectiveDate.IsZero() {
		effectiveDate = time.Now().UTC()
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	var selectedNodeID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("node_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			selectedNodeID = &parsed
		}
	}

	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		errs = append(errs, err.Error())
	}
	tree := mappers.HierarchyToTree(nodes, selectedNodeID)
	parentLabelByID := hierarchyNodeLabelMap(nodes)
	parentIDByNodeID := hierarchyParentIDMap(nodes)

	var selected *viewmodels.OrgNodeDetails
	if selectedNodeID != nil {
		details, err := c.getNodeDetails(r, tenantID, *selectedNodeID, effectiveDate)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			if details != nil {
				details.LongName = c.orgNodeLongNameFor(r, tenantID, *selectedNodeID, effectiveDate)
			}
			if details != nil {
				if parentID, ok := parentIDByNodeID[*selectedNodeID]; ok && parentID != uuid.Nil {
					if label, ok := parentLabelByID[parentID]; ok && strings.TrimSpace(label) != "" {
						details.ParentLabel = label
					} else {
						details.ParentLabel = c.orgNodeLabelFor(r, tenantID, parentID, effectiveDate)
					}
				} else if details.ParentHint != nil && *details.ParentHint != uuid.Nil {
					if label, ok := parentLabelByID[*details.ParentHint]; ok && strings.TrimSpace(label) != "" {
						details.ParentLabel = label
					} else {
						details.ParentLabel = c.orgNodeLabelFor(r, tenantID, *details.ParentHint, effectiveDate)
					}
				}
			}
			selected = details
		}
	}

	props := orgtemplates.NodesPageProps{
		EffectiveDate: effectiveDateStr,
		Tree:          tree,
		SelectedNode:  selected,
		Errors:        errs,
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	templ.Handler(orgtemplates.NodesPage(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) HierarchyPartial(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgHierarchiesAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	var selectedNodeID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("node_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			selectedNodeID = &parsed
		}
	}

	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree := mappers.HierarchyToTree(nodes, selectedNodeID)
	templ.Handler(orgui.Tree(orgui.TreeProps{
		Tree:          tree,
		EffectiveDate: effectiveDateStr,
		SwapOOB:       true,
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) AssignmentsPage(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "assign")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")

	statusCode := http.StatusOK
	var pageErrs []string
	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil {
		statusCode = http.StatusBadRequest
		pageErrs = append(pageErrs, "invalid effective_date")
		effectiveDate = normalizeValidTimeDayUTC(time.Now().UTC())
	}
	if effectiveDate.IsZero() {
		effectiveDate = normalizeValidTimeDayUTC(time.Now().UTC())
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	pernr := strings.TrimSpace(r.URL.Query().Get("pernr"))
	if pernr == "" {
		pernr = strings.TrimSpace(r.URL.Query().Get("Pernr"))
	}
	if pernr == "" {
		pernr = strings.TrimSpace(r.FormValue("pernr"))
	}

	var timeline *viewmodels.OrgAssignmentsTimeline
	if pernr != "" {
		subject := fmt.Sprintf("person:%s", pernr)
		_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
		if err != nil {
			var svcErr *services.ServiceError
			if errors.As(err, &svcErr) {
				http.Error(w, svcErr.Message, svcErr.Status)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		timeline = mappers.AssignmentsToTimeline(subject, rows)
		c.hydrateAssignmentsTimelineLabels(r, tenantID, timeline, effectiveDate)
	}

	if htmx.IsHxRequest(r) && htmx.Target(r) == "org-assignments-timeline" {
		swapSummary := strings.TrimSpace(param(r, "include_summary")) == "1"
		templ.Handler(orgui.AssignmentsTimeline(orgui.AssignmentsTimelineProps{
			EffectiveDate: effectiveDateStr,
			Pernr:         pernr,
			Timeline:      timeline,
			SwapSummary:   swapSummary,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	props := orgtemplates.AssignmentsPageProps{
		EffectiveDate: effectiveDateStr,
		Pernr:         pernr,
		Timeline:      timeline,
		Errors:        pageErrs,
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	templ.Handler(orgtemplates.AssignmentsPage(props), templ.WithStreaming()).ServeHTTP(w, r)
}

type positionsQuery struct {
	effectiveDateProvided bool
	effectiveDate         time.Time
	effectiveDateStr      string

	nodeID      *uuid.UUID
	q           string
	status      string
	staff       string
	page        int
	limit       int
	showSys     bool
	includeDesc bool
}

func positionsQueryFromRequest(r *http.Request) (positionsQuery, []string) {
	var out positionsQuery

	rawEffective := strings.TrimSpace(r.URL.Query().Get("effective_date"))
	out.effectiveDateProvided = rawEffective != ""

	var errs []string
	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil {
		errs = append(errs, "invalid effective_date")
		effectiveDate = time.Now().UTC()
	}
	if effectiveDate.IsZero() {
		effectiveDate = time.Now().UTC()
	}
	out.effectiveDate = effectiveDate.UTC()
	out.effectiveDateStr = out.effectiveDate.Format("2006-01-02")

	if v := strings.TrimSpace(param(r, "node_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			out.nodeID = &parsed
		}
	}

	out.q = strings.TrimSpace(param(r, "q"))
	out.status = strings.TrimSpace(param(r, "lifecycle_status"))
	out.staff = strings.TrimSpace(param(r, "staffing_state"))

	out.includeDesc = true
	if v := strings.TrimSpace(param(r, "include_descendants")); v != "" {
		out.includeDesc = v != "0" && strings.ToLower(v) != "false"
	}
	out.showSys = false
	if v := strings.TrimSpace(param(r, "show_system")); v != "" {
		out.showSys = v == "1" || strings.ToLower(v) == "true"
	}

	out.page = 1
	if v := strings.TrimSpace(param(r, "page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.page = n
		}
	}
	out.limit = 25
	if v := strings.TrimSpace(param(r, "limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			out.limit = n
		}
	}
	return out, errs
}

func canonicalPositionsURL(q positionsQuery, positionID *uuid.UUID) string {
	v := url.Values{}
	v.Set("effective_date", q.effectiveDateStr)
	if q.nodeID != nil && *q.nodeID != uuid.Nil {
		v.Set("node_id", q.nodeID.String())
	}
	if positionID != nil && *positionID != uuid.Nil {
		v.Set("position_id", positionID.String())
	}
	if strings.TrimSpace(q.q) != "" {
		v.Set("q", q.q)
	}
	if strings.TrimSpace(q.status) != "" {
		v.Set("lifecycle_status", q.status)
	}
	if strings.TrimSpace(q.staff) != "" {
		v.Set("staffing_state", q.staff)
	}
	if q.includeDesc {
		v.Set("include_descendants", "1")
	} else {
		v.Set("include_descendants", "0")
	}
	if q.showSys {
		v.Set("show_system", "1")
	} else {
		v.Set("show_system", "0")
	}
	v.Set("page", strconv.Itoa(q.page))
	v.Set("limit", strconv.Itoa(q.limit))
	return "/org/positions?" + v.Encode()
}

func (c *OrgUIController) PositionsPage(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "write", "admin")

	statusCode := http.StatusOK
	q, qErrs := positionsQueryFromRequest(r)
	if len(qErrs) > 0 {
		statusCode = http.StatusBadRequest
	}
	if !q.effectiveDateProvided && !htmx.IsHxRequest(r) {
		http.Redirect(w, r, canonicalPositionsURL(q, nil), http.StatusFound)
		return
	}
	if !q.effectiveDateProvided && htmx.IsHxRequest(r) {
		htmx.PushUrl(w, canonicalPositionsURL(q, nil))
	}

	var selectedNodeID *uuid.UUID
	if q.nodeID != nil {
		selectedNodeID = q.nodeID
	}

	errs := append([]string{}, qErrs...)
	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", q.effectiveDate)
	if err != nil {
		errs = append(errs, err.Error())
	}
	tree := mappers.HierarchyToTree(nodes, selectedNodeID)

	panelProps, selectedPositionID, timeline, selectedPosition, err := c.buildPositionsPanel(r, tenantID, q, uuid.Nil, nodes)
	if err != nil {
		errs = append(errs, err.Error())
	}

	if v := strings.TrimSpace(param(r, "position_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			selectedPositionID = parsed.String()
			details, tl, err := c.getPositionDetails(r, tenantID, parsed, q.effectiveDate)
			if err != nil {
				errs = append(errs, err.Error())
			} else {
				selectedPosition = details
				timeline = tl
			}
		}
	}

	panelProps.SelectedPositionID = selectedPositionID
	panelProps.SelectedPosition = selectedPosition
	panelProps.Timeline = timeline

	props := orgtemplates.PositionsPageProps{
		EffectiveDate: q.effectiveDateStr,
		Tree:          tree,
		Panel:         panelProps,
		Errors:        errs,
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	templ.Handler(orgtemplates.PositionsPage(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func positionsTreeProps(tree *viewmodels.OrgTree, effectiveDateStr string) orgui.TreeProps {
	return orgui.TreeProps{
		Tree:          tree,
		EffectiveDate: effectiveDateStr,
		SwapOOB:       true,
		NodeGetURL: func(nodeID, effectiveDate string) string {
			return fmt.Sprintf("/org/positions/panel?node_id=%s", nodeID)
		},
		Target:  "#org-positions-panel",
		Swap:    "innerHTML",
		Include: "#org-positions-filters, #effective-date",
		PushURL: "true",
	}
}

func (c *OrgUIController) PositionsPanel(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "write", "admin")

	statusCode := http.StatusOK
	q, qErrs := positionsQueryFromRequest(r)
	if len(qErrs) > 0 {
		statusCode = http.StatusBadRequest
	}
	htmx.PushUrl(w, canonicalPositionsURL(q, nil))
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}

	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", q.effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree := mappers.HierarchyToTree(nodes, q.nodeID)

	panelProps, selectedPositionID, timeline, selectedPosition, err := c.buildPositionsPanel(r, tenantID, q, uuid.Nil, nodes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	panelProps.SelectedPositionID = selectedPositionID
	panelProps.SelectedPosition = selectedPosition
	panelProps.Timeline = timeline
	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgui.PositionsPanel(panelProps).Render(ctx, ww); err != nil {
			return err
		}
		return orgui.Tree(positionsTreeProps(tree, q.effectiveDateStr)).Render(ctx, ww)
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) PositionDetails(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "write", "admin")

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	statusCode := http.StatusOK
	q, qErrs := positionsQueryFromRequest(r)
	if len(qErrs) > 0 {
		statusCode = http.StatusBadRequest
	}
	htmx.PushUrl(w, canonicalPositionsURL(q, &positionID))
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}

	panelProps, _, timeline, details, err := c.buildPositionsPanel(r, tenantID, q, positionID, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	panelProps.SelectedPositionID = positionID.String()
	panelProps.SelectedPosition = details
	panelProps.Timeline = timeline

	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgui.PositionDetails(orgui.PositionDetailsProps{
			EffectiveDate: q.effectiveDateStr,
			NodeID:        uuidPtrString(q.nodeID),
			Position:      details,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `<div id="org-positions-list" class="p-3" hx-swap-oob="true">`); err != nil {
			return err
		}
		if err := orgui.PositionsList(orgui.PositionsListProps{
			EffectiveDate:      q.effectiveDateStr,
			Positions:          panelProps.Positions,
			SelectedPositionID: positionID.String(),
			Page:               q.page,
			Limit:              q.limit,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `</div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `<div id="org-position-timeline" class="p-4" hx-swap-oob="true">`); err != nil {
			return err
		}
		if err := orgui.PositionTimeline(orgui.PositionTimelineProps{Items: timeline}).Render(ctx, ww); err != nil {
			return err
		}
		_, err = io.WriteString(ww, `</div>`)
		return err
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) PositionSearchOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("q"))
	var qStr *string
	if raw != "" {
		qStr = &raw
	}

	var orgNodeID *uuid.UUID
	if rawNode := strings.TrimSpace(param(r, "org_node_id")); rawNode != "" {
		if parsed, err := uuid.Parse(rawNode); err == nil && parsed != uuid.Nil {
			orgNodeID = &parsed
		}
	}
	orgNodeRequired := strings.TrimSpace(param(r, "org_node_required")) == "1"
	if orgNodeRequired && orgNodeID == nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	var lifecycle *string
	if v := strings.TrimSpace(param(r, "lifecycle_status")); v != "" {
		lifecycle = &v
	}
	var staffing *string
	if v := strings.TrimSpace(param(r, "staffing_state")); v != "" {
		staffing = &v
	}

	rows, _, err := c.org.GetPositions(r.Context(), tenantID, services.GetPositionsInput{
		AsOf:            &effectiveDate,
		OrgNodeID:       orgNodeID,
		Q:               qStr,
		LifecycleStatus: lifecycle,
		StaffingState:   staffing,
		Limit:           50,
		Offset:          0,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	options := make([]*base.ComboboxOption, 0, len(rows))
	for _, row := range rows {
		label := strings.TrimSpace(row.Code)
		if row.Title != nil && strings.TrimSpace(*row.Title) != "" {
			label = fmt.Sprintf("%s — %s", label, strings.TrimSpace(*row.Title))
		}
		options = append(options, &base.ComboboxOption{Value: row.PositionID.String(), Label: label})
		if len(options) >= 50 {
			break
		}
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) JobFamilyGroupOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	includeInactive := strings.TrimSpace(param(r, "include_inactive")) == "1"
	q := strings.ToLower(strings.TrimSpace(param(r, "q")))

	rows, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type opt struct {
		Code string
		Name string
	}
	out := make([]opt, 0, len(rows))
	for _, row := range rows {
		if !includeInactive && !row.IsActive {
			continue
		}
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if q != "" && !strings.Contains(strings.ToLower(code), q) && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		out = append(out, opt{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})

	options := make([]*base.ComboboxOption, 0, minInt(len(out), 50))
	for i, o := range out {
		if i >= 50 {
			break
		}
		label := strings.TrimSpace(o.Code)
		if strings.TrimSpace(o.Name) != "" {
			label = fmt.Sprintf("%s — %s", strings.TrimSpace(o.Code), strings.TrimSpace(o.Name))
		}
		options = append(options, &base.ComboboxOption{Value: o.Code, Label: label})
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) JobFamilyOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	if groupCode == "" {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	includeInactive := strings.TrimSpace(param(r, "include_inactive")) == "1"
	q := strings.ToLower(strings.TrimSpace(param(r, "q")))

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	rows, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type opt struct {
		Code string
		Name string
	}
	out := make([]opt, 0, len(rows))
	for _, row := range rows {
		if !includeInactive && !row.IsActive {
			continue
		}
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if q != "" && !strings.Contains(strings.ToLower(code), q) && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		out = append(out, opt{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})

	options := make([]*base.ComboboxOption, 0, minInt(len(out), 50))
	for i, o := range out {
		if i >= 50 {
			break
		}
		label := strings.TrimSpace(o.Code)
		if strings.TrimSpace(o.Name) != "" {
			label = fmt.Sprintf("%s — %s", strings.TrimSpace(o.Code), strings.TrimSpace(o.Name))
		}
		options = append(options, &base.ComboboxOption{Value: o.Code, Label: label})
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) JobRoleOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	if groupCode == "" || familyCode == "" {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	includeInactive := strings.TrimSpace(param(r, "include_inactive")) == "1"
	q := strings.ToLower(strings.TrimSpace(param(r, "q")))

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var familyID uuid.UUID
	for _, f := range families {
		if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
			familyID = f.ID
			break
		}
	}
	if familyID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	rows, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type opt struct {
		Code string
		Name string
	}
	out := make([]opt, 0, len(rows))
	for _, row := range rows {
		if !includeInactive && !row.IsActive {
			continue
		}
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if q != "" && !strings.Contains(strings.ToLower(code), q) && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		out = append(out, opt{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})

	options := make([]*base.ComboboxOption, 0, minInt(len(out), 50))
	for i, o := range out {
		if i >= 50 {
			break
		}
		label := strings.TrimSpace(o.Code)
		if strings.TrimSpace(o.Name) != "" {
			label = fmt.Sprintf("%s — %s", strings.TrimSpace(o.Code), strings.TrimSpace(o.Name))
		}
		options = append(options, &base.ComboboxOption{Value: o.Code, Label: label})
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) JobLevelOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	roleCode := strings.TrimSpace(param(r, "job_role_code"))
	if groupCode == "" || familyCode == "" || roleCode == "" {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	includeInactive := strings.TrimSpace(param(r, "include_inactive")) == "1"
	q := strings.ToLower(strings.TrimSpace(param(r, "q")))

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var familyID uuid.UUID
	for _, f := range families {
		if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
			familyID = f.ID
			break
		}
	}
	if familyID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	roles, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var roleID uuid.UUID
	for _, rr := range roles {
		if strings.EqualFold(strings.TrimSpace(rr.Code), roleCode) {
			roleID = rr.ID
			break
		}
	}
	if roleID == uuid.Nil {
		templ.Handler(orgui.NodeSearchOptions([]*base.ComboboxOption{}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	rows, err := c.org.ListJobLevels(r.Context(), tenantID, roleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type opt struct {
		Code string
		Name string
	}
	out := make([]opt, 0, len(rows))
	for _, row := range rows {
		if !includeInactive && !row.IsActive {
			continue
		}
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if q != "" && !strings.Contains(strings.ToLower(code), q) && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		out = append(out, opt{Code: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code == out[j].Code {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})

	options := make([]*base.ComboboxOption, 0, minInt(len(out), 50))
	for i, o := range out {
		if i >= 50 {
			break
		}
		label := strings.TrimSpace(o.Code)
		if strings.TrimSpace(o.Name) != "" {
			label = fmt.Sprintf("%s — %s", strings.TrimSpace(o.Code), strings.TrimSpace(o.Name))
		}
		options = append(options, &base.ComboboxOption{Value: o.Code, Label: label})
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func normalizeJobCatalogTab(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", "family-groups":
		return "family-groups"
	case "families", "roles", "levels":
		return v
	default:
		return "family-groups"
	}
}

func canonicalJobCatalogURL(tab, effectiveDateStr, groupCode, familyCode, roleCode string) string {
	v := url.Values{}
	tab = normalizeJobCatalogTab(tab)
	v.Set("tab", tab)
	if strings.TrimSpace(effectiveDateStr) != "" {
		v.Set("effective_date", strings.TrimSpace(effectiveDateStr))
	}
	if strings.TrimSpace(groupCode) != "" {
		v.Set("job_family_group_code", strings.TrimSpace(groupCode))
	}
	if strings.TrimSpace(familyCode) != "" {
		v.Set("job_family_code", strings.TrimSpace(familyCode))
	}
	if strings.TrimSpace(roleCode) != "" {
		v.Set("job_role_code", strings.TrimSpace(roleCode))
	}
	return "/org/job-catalog?" + v.Encode()
}

func redirectUI(w http.ResponseWriter, r *http.Request, url string) {
	if htmx.IsHxRequest(r) {
		htmx.Redirect(w, url)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (c *OrgUIController) JobCatalogPage(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "read") {
		return
	}
	ensureOrgPageCapabilities(r, orgJobCatalogAuthzObject, "admin")

	statusCode := http.StatusOK
	var pageErrs []string

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil {
		statusCode = http.StatusBadRequest
		pageErrs = append(pageErrs, "invalid effective_date")
		effectiveDate = normalizeValidTimeDayUTC(time.Now().UTC())
	}
	if effectiveDate.IsZero() {
		effectiveDate = normalizeValidTimeDayUTC(time.Now().UTC())
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	tab := normalizeJobCatalogTab(param(r, "tab"))
	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	roleCode := strings.TrimSpace(param(r, "job_role_code"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	props, err := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, pageErrs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) CreateJobFamilyGroupUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	code := strings.TrimSpace(param(r, "code"))
	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	_, err = c.org.CreateJobFamilyGroup(r.Context(), tenantID, services.JobFamilyGroupCreate{
		Code:     code,
		Name:     name,
		IsActive: isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, "", "", "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("family-groups", effectiveDateStr, "", "", ""))
}

func (c *OrgUIController) UpdateJobFamilyGroupUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	_, err = c.org.UpdateJobFamilyGroup(r.Context(), tenantID, id, services.JobFamilyGroupUpdate{
		Name:     &name,
		IsActive: &isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, "", "", "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("family-groups", effectiveDateStr, "", "", ""))
}

func (c *OrgUIController) CreateJobFamilyUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	code := strings.TrimSpace(param(r, "code"))
	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, "", "", editID, []string{"invalid job_family_group_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	_, err = c.org.CreateJobFamily(r.Context(), tenantID, services.JobFamilyCreate{
		JobFamilyGroupID: groupID,
		Code:             code,
		Name:             name,
		IsActive:         isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, "", "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("families", effectiveDateStr, groupCode, "", ""))
}

func (c *OrgUIController) UpdateJobFamilyUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))
	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	_, err = c.org.UpdateJobFamily(r.Context(), tenantID, id, services.JobFamilyUpdate{
		Name:     &name,
		IsActive: &isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, "", "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("families", effectiveDateStr, groupCode, "", ""))
}

func (c *OrgUIController) CreateJobRoleUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	code := strings.TrimSpace(param(r, "code"))
	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, "", editID, []string{"invalid job_family_group_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var familyID uuid.UUID
	for _, f := range families {
		if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
			familyID = f.ID
			break
		}
	}
	if familyID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, "", editID, []string{"invalid job_family_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	_, err = c.org.CreateJobRole(r.Context(), tenantID, services.JobRoleCreate{
		JobFamilyID: familyID,
		Code:        code,
		Name:        name,
		IsActive:    isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("roles", effectiveDateStr, groupCode, familyCode, ""))
}

func (c *OrgUIController) UpdateJobRoleUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))
	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(param(r, "name"))
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	_, err = c.org.UpdateJobRole(r.Context(), tenantID, id, services.JobRoleUpdate{
		Name:     &name,
		IsActive: &isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, "", editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("roles", effectiveDateStr, groupCode, familyCode, ""))
}

func (c *OrgUIController) CreateJobLevelUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))

	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	roleCode := strings.TrimSpace(param(r, "job_role_code"))
	code := strings.TrimSpace(param(r, "code"))
	name := strings.TrimSpace(param(r, "name"))
	displayOrderRaw := strings.TrimSpace(param(r, "display_order"))
	displayOrder := 0
	if displayOrderRaw != "" {
		v, err := strconv.Atoi(displayOrderRaw)
		if err != nil {
			http.Error(w, "invalid display_order", http.StatusBadRequest)
			return
		}
		displayOrder = v
	}
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}
	if groupID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, []string{"invalid job_family_group_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var familyID uuid.UUID
	for _, f := range families {
		if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
			familyID = f.ID
			break
		}
	}
	if familyID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, []string{"invalid job_family_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	roles, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var roleID uuid.UUID
	for _, rr := range roles {
		if strings.EqualFold(strings.TrimSpace(rr.Code), roleCode) {
			roleID = rr.ID
			break
		}
	}
	if roleID == uuid.Nil {
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, []string{"invalid job_role_code"})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	_, err = c.org.CreateJobLevel(r.Context(), tenantID, services.JobLevelCreate{
		JobRoleID:    roleID,
		Code:         code,
		Name:         name,
		DisplayOrder: displayOrder,
		IsActive:     isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("levels", effectiveDateStr, groupCode, familyCode, roleCode))
}

func (c *OrgUIController) UpdateJobLevelUI(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgJobCatalogAuthzObject, "admin")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgJobCatalogAuthzObject, "admin") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := normalizeValidTimeDayUTC(effectiveDate).UTC().Format("2006-01-02")
	tab := normalizeJobCatalogTab(param(r, "tab"))
	editID := strings.TrimSpace(param(r, "edit_id"))
	groupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	familyCode := strings.TrimSpace(param(r, "job_family_code"))
	roleCode := strings.TrimSpace(param(r, "job_role_code"))

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(param(r, "name"))
	displayOrderRaw := strings.TrimSpace(param(r, "display_order"))
	displayOrder := 0
	if displayOrderRaw != "" {
		v, err := strconv.Atoi(displayOrderRaw)
		if err != nil {
			http.Error(w, "invalid display_order", http.StatusBadRequest)
			return
		}
		displayOrder = v
	}
	isActive := strings.TrimSpace(param(r, "is_active")) != "0"

	_, err = c.org.UpdateJobLevel(r.Context(), tenantID, id, services.JobLevelUpdate{
		Name:         &name,
		DisplayOrder: &displayOrder,
		IsActive:     &isActive,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		props, buildErr := c.buildJobCatalogPageProps(r, tenantID, effectiveDateStr, tab, groupCode, familyCode, roleCode, editID, []string{formErr})
		if buildErr != nil {
			http.Error(w, buildErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(statusCode)
		templ.Handler(orgtemplates.JobCatalogPage(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}
	redirectUI(w, r, canonicalJobCatalogURL("levels", effectiveDateStr, groupCode, familyCode, roleCode))
}

func (c *OrgUIController) buildJobCatalogPageProps(
	r *http.Request,
	tenantID uuid.UUID,
	effectiveDateStr string,
	tab string,
	groupCode string,
	familyCode string,
	roleCode string,
	editID string,
	pageErrs []string,
) (orgtemplates.JobCatalogPageProps, error) {
	tab = normalizeJobCatalogTab(tab)
	groupCode = strings.TrimSpace(groupCode)
	familyCode = strings.TrimSpace(familyCode)
	roleCode = strings.TrimSpace(roleCode)
	editID = strings.TrimSpace(editID)

	groupLabel, familyLabel, roleLabel, _ := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
		JobFamilyGroupCode: groupCode,
		JobFamilyCode:      familyCode,
		JobRoleCode:        roleCode,
	})

	props := orgtemplates.JobCatalogPageProps{
		EffectiveDate:       effectiveDateStr,
		Tab:                 tab,
		JobFamilyGroupCode:  groupCode,
		JobFamilyGroupLabel: groupLabel,
		JobFamilyCode:       familyCode,
		JobFamilyLabel:      familyLabel,
		JobRoleCode:         roleCode,
		JobRoleLabel:        roleLabel,
		FamilyGroups:        []viewmodels.JobFamilyGroupRow{},
		Families:            []viewmodels.JobFamilyRow{},
		Roles:               []viewmodels.JobRoleRow{},
		Levels:              []viewmodels.JobLevelRow{},
		EditID:              editID,
		Errors:              pageErrs,
	}

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		return props, err
	}

	props.FamilyGroups = make([]viewmodels.JobFamilyGroupRow, 0, len(groups))
	for _, g := range groups {
		props.FamilyGroups = append(props.FamilyGroups, viewmodels.JobFamilyGroupRow{
			ID:       g.ID,
			Code:     strings.TrimSpace(g.Code),
			Name:     strings.TrimSpace(g.Name),
			IsActive: g.IsActive,
		})
	}

	var groupID uuid.UUID
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), groupCode) {
			groupID = g.ID
			break
		}
	}

	switch tab {
	case "family-groups":
		if editID != "" {
			if parsed, err := uuid.Parse(editID); err == nil && parsed != uuid.Nil {
				for _, row := range props.FamilyGroups {
					if row.ID == parsed {
						selected := row
						props.EditFamilyGroup = &selected
						break
					}
				}
			}
		}
	case "families":
		if groupID == uuid.Nil {
			return props, nil
		}
		rows, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
		if err != nil {
			return props, err
		}
		props.Families = make([]viewmodels.JobFamilyRow, 0, len(rows))
		for _, rr := range rows {
			props.Families = append(props.Families, viewmodels.JobFamilyRow{
				ID:               rr.ID,
				JobFamilyGroupID: rr.JobFamilyGroupID,
				Code:             strings.TrimSpace(rr.Code),
				Name:             strings.TrimSpace(rr.Name),
				IsActive:         rr.IsActive,
			})
		}
		if editID != "" {
			if parsed, err := uuid.Parse(editID); err == nil && parsed != uuid.Nil {
				for _, row := range props.Families {
					if row.ID == parsed {
						selected := row
						props.EditFamily = &selected
						break
					}
				}
			}
		}
	case "roles":
		if groupID == uuid.Nil || familyCode == "" {
			return props, nil
		}
		families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
		if err != nil {
			return props, err
		}
		var familyID uuid.UUID
		for _, f := range families {
			if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
				familyID = f.ID
				break
			}
		}
		if familyID == uuid.Nil {
			return props, nil
		}
		rows, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
		if err != nil {
			return props, err
		}
		props.Roles = make([]viewmodels.JobRoleRow, 0, len(rows))
		for _, rr := range rows {
			props.Roles = append(props.Roles, viewmodels.JobRoleRow{
				ID:          rr.ID,
				JobFamilyID: rr.JobFamilyID,
				Code:        strings.TrimSpace(rr.Code),
				Name:        strings.TrimSpace(rr.Name),
				IsActive:    rr.IsActive,
			})
		}
		if editID != "" {
			if parsed, err := uuid.Parse(editID); err == nil && parsed != uuid.Nil {
				for _, row := range props.Roles {
					if row.ID == parsed {
						selected := row
						props.EditRole = &selected
						break
					}
				}
			}
		}
	case "levels":
		if groupID == uuid.Nil || familyCode == "" || roleCode == "" {
			return props, nil
		}
		families, err := c.org.ListJobFamilies(r.Context(), tenantID, groupID)
		if err != nil {
			return props, err
		}
		var familyID uuid.UUID
		for _, f := range families {
			if strings.EqualFold(strings.TrimSpace(f.Code), familyCode) {
				familyID = f.ID
				break
			}
		}
		if familyID == uuid.Nil {
			return props, nil
		}
		roles, err := c.org.ListJobRoles(r.Context(), tenantID, familyID)
		if err != nil {
			return props, err
		}
		var roleID uuid.UUID
		for _, rr := range roles {
			if strings.EqualFold(strings.TrimSpace(rr.Code), roleCode) {
				roleID = rr.ID
				break
			}
		}
		if roleID == uuid.Nil {
			return props, nil
		}
		rows, err := c.org.ListJobLevels(r.Context(), tenantID, roleID)
		if err != nil {
			return props, err
		}
		props.Levels = make([]viewmodels.JobLevelRow, 0, len(rows))
		for _, rr := range rows {
			props.Levels = append(props.Levels, viewmodels.JobLevelRow{
				ID:           rr.ID,
				JobRoleID:    rr.JobRoleID,
				Code:         strings.TrimSpace(rr.Code),
				Name:         strings.TrimSpace(rr.Name),
				DisplayOrder: rr.DisplayOrder,
				IsActive:     rr.IsActive,
			})
		}
		if editID != "" {
			if parsed, err := uuid.Parse(editID); err == nil && parsed != uuid.Nil {
				for _, row := range props.Levels {
					if row.ID == parsed {
						selected := row
						props.EditLevel = &selected
						break
					}
				}
			}
		}
	}

	return props, nil
}

func (c *OrgUIController) NewPositionForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	nodeID := strings.TrimSpace(param(r, "node_id"))
	nodeLabel := ""
	if parsed, err := uuid.Parse(nodeID); err == nil {
		nodeLabel = c.orgNodeLabelFor(r, tenantID, parsed, effectiveDate)
	}
	templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
		Mode:                orgui.PositionFormCreate,
		EffectiveDate:       effectiveDateStr,
		NodeID:              nodeID,
		Code:                "",
		OrgNodeID:           nodeID,
		OrgNodeLabel:        nodeLabel,
		LifecycleStatus:     "active",
		PositionType:        "regular",
		EmploymentType:      "full_time",
		CapacityFTE:         "1.00",
		ReasonCode:          "create",
		JobFamilyGroupCode:  "",
		JobFamilyGroupLabel: "",
		JobFamilyCode:       "",
		JobFamilyLabel:      "",
		JobRoleCode:         "",
		JobRoleLabel:        "",
		JobLevelCode:        "",
		JobLevelLabel:       "",
		Errors:              map[string]string{},
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) CreatePosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	code := strings.TrimSpace(param(r, "code"))
	orgNodeRaw := strings.TrimSpace(param(r, "org_node_id"))
	orgNodeID, err := uuid.Parse(orgNodeRaw)
	if err != nil {
		http.Error(w, "invalid org_node_id", http.StatusBadRequest)
		return
	}
	titleRaw := strings.TrimSpace(param(r, "title"))
	var title *string
	if titleRaw != "" {
		title = &titleRaw
	}
	lifecycle := strings.TrimSpace(param(r, "lifecycle_status"))
	capacity := 1.0
	if raw := strings.TrimSpace(param(r, "capacity_fte")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			http.Error(w, "invalid capacity_fte", http.StatusBadRequest)
			return
		}
		capacity = v
	}
	var reportsTo *uuid.UUID
	if raw := strings.TrimSpace(param(r, "reports_to_position_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid reports_to_position_id", http.StatusBadRequest)
			return
		}
		reportsTo = &id
	}
	reasonCode := strings.TrimSpace(param(r, "reason_code"))
	reasonNoteRaw := strings.TrimSpace(param(r, "reason_note"))
	var reasonNote *string
	if reasonNoteRaw != "" {
		reasonNote = &reasonNoteRaw
	}

	positionType := strings.TrimSpace(param(r, "position_type"))
	employmentType := strings.TrimSpace(param(r, "employment_type"))
	jobFamilyGroupCode := strings.TrimSpace(param(r, "job_family_group_code"))
	jobFamilyCode := strings.TrimSpace(param(r, "job_family_code"))
	jobRoleCode := strings.TrimSpace(param(r, "job_role_code"))
	jobLevelCode := strings.TrimSpace(param(r, "job_level_code"))

	fieldErrs := map[string]string{}
	if positionType == "" {
		fieldErrs["position_type"] = "required"
	}
	if employmentType == "" {
		fieldErrs["employment_type"] = "required"
	}
	if jobFamilyGroupCode == "" {
		fieldErrs["job_family_group_code"] = "required"
	}
	if jobFamilyCode == "" {
		fieldErrs["job_family_code"] = "required"
	}
	if jobRoleCode == "" {
		fieldErrs["job_role_code"] = "required"
	}
	if jobLevelCode == "" {
		fieldErrs["job_level_code"] = "required"
	}
	if len(fieldErrs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		nodeLabel := c.orgNodeLabelFor(r, tenantID, orgNodeID, effectiveDate)
		groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
			JobFamilyGroupCode: jobFamilyGroupCode,
			JobFamilyCode:      jobFamilyCode,
			JobRoleCode:        jobRoleCode,
			JobLevelCode:       jobLevelCode,
		})
		templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
			Mode:                orgui.PositionFormCreate,
			EffectiveDate:       effectiveDateStr,
			NodeID:              orgNodeID.String(),
			Code:                code,
			OrgNodeID:           orgNodeID.String(),
			OrgNodeLabel:        nodeLabel,
			Title:               titleRaw,
			LifecycleStatus:     lifecycle,
			PositionType:        positionType,
			EmploymentType:      employmentType,
			CapacityFTE:         fmt.Sprintf("%.2f", capacity),
			ReasonCode:          reasonCode,
			ReasonNote:          reasonNoteRaw,
			JobFamilyGroupCode:  jobFamilyGroupCode,
			JobFamilyGroupLabel: groupLabel,
			JobFamilyCode:       jobFamilyCode,
			JobFamilyLabel:      familyLabel,
			JobRoleCode:         jobRoleCode,
			JobRoleLabel:        roleLabel,
			JobLevelCode:        jobLevelCode,
			JobLevelLabel:       levelLabel,
			Errors:              fieldErrs,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	res, err := c.org.CreatePosition(r.Context(), tenantID, requestID, initiatorID, services.CreatePositionInput{
		Code:               code,
		OrgNodeID:          orgNodeID,
		EffectiveDate:      effectiveDate,
		Title:              title,
		LifecycleStatus:    lifecycle,
		PositionType:       positionType,
		EmploymentType:     employmentType,
		CapacityFTE:        capacity,
		ReportsToID:        reportsTo,
		JobFamilyGroupCode: jobFamilyGroupCode,
		JobFamilyCode:      jobFamilyCode,
		JobRoleCode:        jobRoleCode,
		JobLevelCode:       jobLevelCode,
		ReasonCode:         reasonCode,
		ReasonNote:         reasonNote,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		formErr = attachRequestID(formErr, requestID)
		w.WriteHeader(statusCode)
		nodeLabel := c.orgNodeLabelFor(r, tenantID, orgNodeID, effectiveDate)
		groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
			JobFamilyGroupCode: jobFamilyGroupCode,
			JobFamilyCode:      jobFamilyCode,
			JobRoleCode:        jobRoleCode,
			JobLevelCode:       jobLevelCode,
		})
		templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
			Mode:                orgui.PositionFormCreate,
			EffectiveDate:       effectiveDateStr,
			NodeID:              orgNodeID.String(),
			Code:                code,
			OrgNodeID:           orgNodeID.String(),
			OrgNodeLabel:        nodeLabel,
			Title:               titleRaw,
			LifecycleStatus:     lifecycle,
			PositionType:        positionType,
			EmploymentType:      employmentType,
			CapacityFTE:         fmt.Sprintf("%.2f", capacity),
			ReasonCode:          reasonCode,
			ReasonNote:          reasonNoteRaw,
			JobFamilyGroupCode:  jobFamilyGroupCode,
			JobFamilyGroupLabel: groupLabel,
			JobFamilyCode:       jobFamilyCode,
			JobFamilyLabel:      familyLabel,
			JobRoleCode:         jobRoleCode,
			JobRoleLabel:        roleLabel,
			JobLevelCode:        jobLevelCode,
			JobLevelLabel:       levelLabel,
			Errors:              map[string]string{},
			FormError:           formErr,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	q, _ := positionsQueryFromRequest(r)
	q.effectiveDate = effectiveDate.UTC()
	q.effectiveDateStr = effectiveDateStr
	q.nodeID = &orgNodeID
	htmx.PushUrl(w, canonicalPositionsURL(q, &res.PositionID))

	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", q.effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree := mappers.HierarchyToTree(nodes, q.nodeID)

	panelProps, selectedPositionID, timeline, details, err := c.buildPositionsPanel(r, tenantID, q, res.PositionID, nodes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	panelProps.SelectedPositionID = selectedPositionID
	panelProps.SelectedPosition = details
	panelProps.Timeline = timeline

	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgtemplates.PositionsHeader(orgtemplates.PositionsHeaderProps{
			EffectiveDate: effectiveDateStr,
			SwapOOB:       true,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if err := orgui.PositionDetails(orgui.PositionDetailsProps{
			EffectiveDate: effectiveDateStr,
			NodeID:        orgNodeID.String(),
			Position:      details,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `<div id="org-positions-panel" hx-swap-oob="true">`); err != nil {
			return err
		}
		if err := orgui.PositionsPanel(panelProps).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `</div>`); err != nil {
			return err
		}
		return orgui.Tree(positionsTreeProps(tree, effectiveDateStr)).Render(ctx, ww)
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) EditPositionForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	row, _, err := c.org.GetPosition(r.Context(), tenantID, positionID, &effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slices, err := c.org.GetPositionTimeline(r.Context(), tenantID, positionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sliceAt := mappers.FindPositionSliceAt(slices, effectiveDate)
	reportsToID := ""
	reportsToLabel := ""
	if sliceAt != nil && sliceAt.ReportsToPositionID != nil && *sliceAt.ReportsToPositionID != uuid.Nil {
		reportsToID = sliceAt.ReportsToPositionID.String()
		reportsToLabel = c.positionLabelFor(r, tenantID, *sliceAt.ReportsToPositionID, effectiveDate, reportsToID)
	}
	nodeLabel := c.orgNodeLabelFor(r, tenantID, row.OrgNodeID, effectiveDate)
	title := ""
	if row.Title != nil {
		title = strings.TrimSpace(*row.Title)
	}
	positionType := ""
	if row.PositionType != nil {
		positionType = strings.TrimSpace(*row.PositionType)
	}
	employmentType := ""
	if row.EmploymentType != nil {
		employmentType = strings.TrimSpace(*row.EmploymentType)
	}
	jobFamilyGroupCode := ""
	if row.JobFamilyGroupCode != nil {
		jobFamilyGroupCode = strings.TrimSpace(*row.JobFamilyGroupCode)
	}
	jobFamilyCode := ""
	if row.JobFamilyCode != nil {
		jobFamilyCode = strings.TrimSpace(*row.JobFamilyCode)
	}
	jobRoleCode := ""
	if row.JobRoleCode != nil {
		jobRoleCode = strings.TrimSpace(*row.JobRoleCode)
	}
	jobLevelCode := ""
	if row.JobLevelCode != nil {
		jobLevelCode = strings.TrimSpace(*row.JobLevelCode)
	}
	groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
		JobFamilyGroupCode: jobFamilyGroupCode,
		JobFamilyCode:      jobFamilyCode,
		JobRoleCode:        jobRoleCode,
		JobLevelCode:       jobLevelCode,
	})
	templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
		Mode:                orgui.PositionFormEdit,
		EffectiveDate:       effectiveDateStr,
		NodeID:              strings.TrimSpace(param(r, "node_id")),
		PositionID:          positionID.String(),
		Code:                row.Code,
		OrgNodeID:           row.OrgNodeID.String(),
		OrgNodeLabel:        nodeLabel,
		Title:               title,
		LifecycleStatus:     row.LifecycleStatus,
		PositionType:        positionType,
		EmploymentType:      employmentType,
		CapacityFTE:         fmt.Sprintf("%.2f", row.CapacityFTE),
		ReportsToID:         reportsToID,
		ReportsToLabel:      reportsToLabel,
		ReasonCode:          "update",
		ReasonNote:          "",
		JobFamilyGroupCode:  jobFamilyGroupCode,
		JobFamilyGroupLabel: groupLabel,
		JobFamilyCode:       jobFamilyCode,
		JobFamilyLabel:      familyLabel,
		JobRoleCode:         jobRoleCode,
		JobRoleLabel:        roleLabel,
		JobLevelCode:        jobLevelCode,
		JobLevelLabel:       levelLabel,
		Errors:              map[string]string{},
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgPositionsAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgPositionsAuthzObject, "write") {
		return
	}
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	positionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	orgNodeRaw := strings.TrimSpace(param(r, "org_node_id"))
	orgNodeID, err := uuid.Parse(orgNodeRaw)
	if err != nil {
		http.Error(w, "invalid org_node_id", http.StatusBadRequest)
		return
	}
	titleRaw := strings.TrimSpace(param(r, "title"))
	var title *string
	if titleRaw != "" {
		title = &titleRaw
	}
	lifecycleRaw := strings.TrimSpace(param(r, "lifecycle_status"))
	var lifecycle *string
	if lifecycleRaw != "" {
		lifecycle = &lifecycleRaw
	}
	capacityRaw := strings.TrimSpace(param(r, "capacity_fte"))
	var capacity *float64
	if capacityRaw != "" {
		v, err := strconv.ParseFloat(capacityRaw, 64)
		if err != nil {
			http.Error(w, "invalid capacity_fte", http.StatusBadRequest)
			return
		}
		capacity = &v
	}
	var reportsTo *uuid.UUID
	if raw := strings.TrimSpace(param(r, "reports_to_position_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid reports_to_position_id", http.StatusBadRequest)
			return
		}
		reportsTo = &id
	}
	reasonCode := strings.TrimSpace(param(r, "reason_code"))
	reasonNoteRaw := strings.TrimSpace(param(r, "reason_note"))
	var reasonNote *string
	if reasonNoteRaw != "" {
		reasonNote = &reasonNoteRaw
	}

	positionTypeRaw := strings.TrimSpace(param(r, "position_type"))
	employmentTypeRaw := strings.TrimSpace(param(r, "employment_type"))
	jobFamilyGroupCodeRaw := strings.TrimSpace(param(r, "job_family_group_code"))
	jobFamilyCodeRaw := strings.TrimSpace(param(r, "job_family_code"))
	jobRoleCodeRaw := strings.TrimSpace(param(r, "job_role_code"))
	jobLevelCodeRaw := strings.TrimSpace(param(r, "job_level_code"))

	fieldErrs := map[string]string{}
	if positionTypeRaw == "" {
		fieldErrs["position_type"] = "required"
	}
	if employmentTypeRaw == "" {
		fieldErrs["employment_type"] = "required"
	}
	if jobFamilyGroupCodeRaw == "" {
		fieldErrs["job_family_group_code"] = "required"
	}
	if jobFamilyCodeRaw == "" {
		fieldErrs["job_family_code"] = "required"
	}
	if jobRoleCodeRaw == "" {
		fieldErrs["job_role_code"] = "required"
	}
	if jobLevelCodeRaw == "" {
		fieldErrs["job_level_code"] = "required"
	}
	if len(fieldErrs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		nodeLabel := c.orgNodeLabelFor(r, tenantID, orgNodeID, effectiveDate)
		reportsToLabel := strings.TrimSpace(param(r, "reports_to_position_id"))
		if reportsToLabel != "" {
			if parsed, err := uuid.Parse(reportsToLabel); err == nil && parsed != uuid.Nil {
				reportsToLabel = c.positionLabelFor(r, tenantID, parsed, effectiveDate, reportsToLabel)
			}
		}
		groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
			JobFamilyGroupCode: jobFamilyGroupCodeRaw,
			JobFamilyCode:      jobFamilyCodeRaw,
			JobRoleCode:        jobRoleCodeRaw,
			JobLevelCode:       jobLevelCodeRaw,
		})
		templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
			Mode:                orgui.PositionFormEdit,
			EffectiveDate:       effectiveDateStr,
			NodeID:              strings.TrimSpace(param(r, "node_id")),
			PositionID:          positionID.String(),
			Code:                strings.TrimSpace(param(r, "code")),
			OrgNodeID:           orgNodeID.String(),
			OrgNodeLabel:        nodeLabel,
			Title:               titleRaw,
			LifecycleStatus:     lifecycleRaw,
			PositionType:        positionTypeRaw,
			EmploymentType:      employmentTypeRaw,
			CapacityFTE:         capacityRaw,
			ReportsToID:         strings.TrimSpace(param(r, "reports_to_position_id")),
			ReportsToLabel:      reportsToLabel,
			ReasonCode:          reasonCode,
			ReasonNote:          reasonNoteRaw,
			JobFamilyGroupCode:  jobFamilyGroupCodeRaw,
			JobFamilyGroupLabel: groupLabel,
			JobFamilyCode:       jobFamilyCodeRaw,
			JobFamilyLabel:      familyLabel,
			JobRoleCode:         jobRoleCodeRaw,
			JobRoleLabel:        roleLabel,
			JobLevelCode:        jobLevelCodeRaw,
			JobLevelLabel:       levelLabel,
			Errors:              fieldErrs,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	positionType := positionTypeRaw
	employmentType := employmentTypeRaw
	jobFamilyGroupCode := jobFamilyGroupCodeRaw
	jobFamilyCode := jobFamilyCodeRaw
	jobRoleCode := jobRoleCodeRaw
	jobLevelCode := jobLevelCodeRaw

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.UpdatePosition(r.Context(), tenantID, requestID, initiatorID, services.UpdatePositionInput{
		PositionID:         positionID,
		EffectiveDate:      effectiveDate,
		ReasonCode:         reasonCode,
		ReasonNote:         reasonNote,
		OrgNodeID:          &orgNodeID,
		Title:              title,
		LifecycleStatus:    lifecycle,
		PositionType:       &positionType,
		EmploymentType:     &employmentType,
		JobFamilyGroupCode: &jobFamilyGroupCode,
		JobFamilyCode:      &jobFamilyCode,
		JobRoleCode:        &jobRoleCode,
		JobLevelCode:       &jobLevelCode,
		CapacityFTE:        capacity,
		ReportsToID:        reportsTo,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		formErr = attachRequestID(formErr, requestID)
		w.WriteHeader(statusCode)
		nodeLabel := c.orgNodeLabelFor(r, tenantID, orgNodeID, effectiveDate)
		reportsToIDRaw := strings.TrimSpace(param(r, "reports_to_position_id"))
		reportsToLabel := reportsToIDRaw
		if reportsToIDRaw != "" {
			if parsed, err := uuid.Parse(reportsToIDRaw); err == nil && parsed != uuid.Nil {
				reportsToLabel = c.positionLabelFor(r, tenantID, parsed, effectiveDate, reportsToIDRaw)
			}
		}
		groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
			JobFamilyGroupCode: jobFamilyGroupCodeRaw,
			JobFamilyCode:      jobFamilyCodeRaw,
			JobRoleCode:        jobRoleCodeRaw,
			JobLevelCode:       jobLevelCodeRaw,
		})
		templ.Handler(orgui.PositionForm(orgui.PositionFormProps{
			Mode:                orgui.PositionFormEdit,
			EffectiveDate:       effectiveDateStr,
			NodeID:              strings.TrimSpace(param(r, "node_id")),
			PositionID:          positionID.String(),
			Code:                strings.TrimSpace(param(r, "code")),
			OrgNodeID:           orgNodeID.String(),
			OrgNodeLabel:        nodeLabel,
			Title:               titleRaw,
			LifecycleStatus:     lifecycleRaw,
			PositionType:        positionTypeRaw,
			EmploymentType:      employmentTypeRaw,
			CapacityFTE:         capacityRaw,
			ReportsToID:         reportsToIDRaw,
			ReportsToLabel:      reportsToLabel,
			ReasonCode:          reasonCode,
			ReasonNote:          reasonNoteRaw,
			JobFamilyGroupCode:  jobFamilyGroupCodeRaw,
			JobFamilyGroupLabel: groupLabel,
			JobFamilyCode:       jobFamilyCodeRaw,
			JobFamilyLabel:      familyLabel,
			JobRoleCode:         jobRoleCodeRaw,
			JobRoleLabel:        roleLabel,
			JobLevelCode:        jobLevelCodeRaw,
			JobLevelLabel:       levelLabel,
			Errors:              map[string]string{},
			FormError:           formErr,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	q, _ := positionsQueryFromRequest(r)
	q.effectiveDate = effectiveDate.UTC()
	q.effectiveDateStr = effectiveDateStr
	q.nodeID = &orgNodeID
	htmx.PushUrl(w, canonicalPositionsURL(q, &positionID))

	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", q.effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree := mappers.HierarchyToTree(nodes, q.nodeID)

	panelProps, selectedPositionID, timeline, details, err := c.buildPositionsPanel(r, tenantID, q, positionID, nodes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	panelProps.SelectedPositionID = selectedPositionID
	panelProps.SelectedPosition = details
	panelProps.Timeline = timeline

	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgtemplates.PositionsHeader(orgtemplates.PositionsHeaderProps{
			EffectiveDate: effectiveDateStr,
			SwapOOB:       true,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if err := orgui.PositionDetails(orgui.PositionDetailsProps{
			EffectiveDate: effectiveDateStr,
			NodeID:        orgNodeID.String(),
			Position:      details,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `<div id="org-positions-panel" hx-swap-oob="true">`); err != nil {
			return err
		}
		if err := orgui.PositionsPanel(panelProps).Render(ctx, ww); err != nil {
			return err
		}
		if _, err := io.WriteString(ww, `</div>`); err != nil {
			return err
		}
		return orgui.Tree(positionsTreeProps(tree, effectiveDateStr)).Render(ctx, ww)
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) buildPositionsPanel(r *http.Request, tenantID uuid.UUID, q positionsQuery, selectedPositionID uuid.UUID, hierarchyNodes []services.HierarchyNode) (orgui.PositionsPanelProps, string, []viewmodels.OrgPositionTimelineItem, *viewmodels.OrgPositionDetails, error) {
	out := orgui.PositionsPanelProps{
		EffectiveDate:      q.effectiveDateStr,
		NodeID:             uuidPtrString(q.nodeID),
		Q:                  q.q,
		LifecycleStatus:    q.status,
		StaffingState:      q.staff,
		ShowSystem:         q.showSys,
		IncludeDescendants: q.includeDesc,
		Page:               q.page,
		Limit:              q.limit,
		Positions:          []viewmodels.OrgPositionRow{},
		SelectedPositionID: "",
		SelectedPosition:   nil,
		Timeline:           nil,
	}

	if q.nodeID == nil || *q.nodeID == uuid.Nil {
		return out, "", nil, nil, nil
	}

	var orgNodeIDs []uuid.UUID
	if q.includeDesc {
		nodes := hierarchyNodes
		if nodes == nil {
			fetched, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", q.effectiveDate)
			if err != nil {
				return out, "", nil, nil, err
			}
			nodes = fetched
		}
		orgNodeIDs = descendantNodeIDs(nodes, *q.nodeID)
	} else {
		orgNodeIDs = []uuid.UUID{*q.nodeID}
	}

	var qPtr *string
	if strings.TrimSpace(q.q) != "" {
		v := q.q
		qPtr = &v
	}
	var lifecyclePtr *string
	if strings.TrimSpace(q.status) != "" {
		v := q.status
		lifecyclePtr = &v
	}
	var staffPtr *string
	if strings.TrimSpace(q.staff) != "" {
		v := q.staff
		staffPtr = &v
	}

	var isAutoCreated *bool
	if !q.showSys {
		v := false
		isAutoCreated = &v
	}
	offset := (q.page - 1) * q.limit

	rows, _, err := c.org.GetPositions(r.Context(), tenantID, services.GetPositionsInput{
		AsOf:            &q.effectiveDate,
		OrgNodeIDs:      orgNodeIDs,
		Q:               qPtr,
		LifecycleStatus: lifecyclePtr,
		StaffingState:   staffPtr,
		IsAutoCreated:   isAutoCreated,
		Limit:           q.limit,
		Offset:          offset,
	})
	if err != nil {
		return out, "", nil, nil, err
	}
	out.Positions = mappers.PositionsToViewModels(rows)

	if selectedPositionID == uuid.Nil {
		return out, "", nil, nil, nil
	}
	details, timeline, err := c.getPositionDetails(r, tenantID, selectedPositionID, q.effectiveDate)
	if err != nil {
		return out, "", nil, nil, err
	}
	return out, selectedPositionID.String(), timeline, details, nil
}

func (c *OrgUIController) getPositionDetails(r *http.Request, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time) (*viewmodels.OrgPositionDetails, []viewmodels.OrgPositionTimelineItem, error) {
	row, _, err := c.org.GetPosition(r.Context(), tenantID, positionID, &asOf)
	if err != nil {
		return nil, nil, err
	}
	slices, err := c.org.GetPositionTimeline(r.Context(), tenantID, positionID)
	if err != nil {
		return nil, nil, err
	}
	sliceAt := mappers.FindPositionSliceAt(slices, asOf)
	var reportsTo *uuid.UUID
	if sliceAt != nil {
		reportsTo = sliceAt.ReportsToPositionID
	}
	details := mappers.PositionDetailsFrom(row, reportsTo)
	if details != nil && details.ReportsToPositionID != nil && *details.ReportsToPositionID != uuid.Nil {
		details.ReportsToLabel = c.positionLabelFor(r, tenantID, *details.ReportsToPositionID, asOf, details.ReportsToPositionID.String())
	}
	if details != nil {
		groupLabel, familyLabel, roleLabel, levelLabel := c.jobCatalogLabelsFor(r, tenantID, services.JobCatalogCodes{
			JobFamilyGroupCode: details.Row.JobFamilyGroupCode,
			JobFamilyCode:      details.Row.JobFamilyCode,
			JobRoleCode:        details.Row.JobRoleCode,
			JobLevelCode:       details.Row.JobLevelCode,
		})
		details.JobFamilyGroupLabel = groupLabel
		details.JobFamilyLabel = familyLabel
		details.JobRoleLabel = roleLabel
		details.JobLevelLabel = levelLabel
	}
	return details, mappers.PositionTimelineToViewModels(slices), nil
}

func (c *OrgUIController) jobCatalogLabelsFor(r *http.Request, tenantID uuid.UUID, codes services.JobCatalogCodes) (string, string, string, string) {
	codes.JobFamilyGroupCode = strings.TrimSpace(codes.JobFamilyGroupCode)
	codes.JobFamilyCode = strings.TrimSpace(codes.JobFamilyCode)
	codes.JobRoleCode = strings.TrimSpace(codes.JobRoleCode)
	codes.JobLevelCode = strings.TrimSpace(codes.JobLevelCode)

	if codes.JobFamilyGroupCode == "" && codes.JobFamilyCode == "" && codes.JobRoleCode == "" && codes.JobLevelCode == "" {
		return "", "", "", ""
	}

	groups, err := c.org.ListJobFamilyGroups(r.Context(), tenantID)
	if err != nil {
		return "", "", "", ""
	}
	var group *services.JobFamilyGroupRow
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Code), codes.JobFamilyGroupCode) {
			found := g
			group = &found
			break
		}
	}
	if group == nil {
		return "", "", "", ""
	}
	groupLabel := strings.TrimSpace(group.Name)
	if codes.JobFamilyCode == "" {
		return groupLabel, "", "", ""
	}

	families, err := c.org.ListJobFamilies(r.Context(), tenantID, group.ID)
	if err != nil {
		return groupLabel, "", "", ""
	}
	var family *services.JobFamilyRow
	for _, f := range families {
		if strings.EqualFold(strings.TrimSpace(f.Code), codes.JobFamilyCode) {
			found := f
			family = &found
			break
		}
	}
	if family == nil {
		return groupLabel, "", "", ""
	}
	familyLabel := strings.TrimSpace(family.Name)
	if codes.JobRoleCode == "" {
		return groupLabel, familyLabel, "", ""
	}

	roles, err := c.org.ListJobRoles(r.Context(), tenantID, family.ID)
	if err != nil {
		return groupLabel, familyLabel, "", ""
	}
	var role *services.JobRoleRow
	for _, rr := range roles {
		if strings.EqualFold(strings.TrimSpace(rr.Code), codes.JobRoleCode) {
			found := rr
			role = &found
			break
		}
	}
	if role == nil {
		return groupLabel, familyLabel, "", ""
	}
	roleLabel := strings.TrimSpace(role.Name)
	if codes.JobLevelCode == "" {
		return groupLabel, familyLabel, roleLabel, ""
	}

	levels, err := c.org.ListJobLevels(r.Context(), tenantID, role.ID)
	if err != nil {
		return groupLabel, familyLabel, roleLabel, ""
	}
	for _, lv := range levels {
		if strings.EqualFold(strings.TrimSpace(lv.Code), codes.JobLevelCode) {
			return groupLabel, familyLabel, roleLabel, strings.TrimSpace(lv.Name)
		}
	}
	return groupLabel, familyLabel, roleLabel, ""
}

func descendantNodeIDs(nodes []services.HierarchyNode, root uuid.UUID) []uuid.UUID {
	children := map[uuid.UUID][]uuid.UUID{}
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == uuid.Nil {
			continue
		}
		children[*n.ParentID] = append(children[*n.ParentID], n.ID)
	}
	out := make([]uuid.UUID, 0, 32)
	queue := []uuid.UUID{root}
	seen := map[uuid.UUID]bool{root: true}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, child := range children[id] {
			if seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
		}
	}
	return out
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil || *id == uuid.Nil {
		return ""
	}
	return id.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func attachRequestID(msg, requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return msg
	}
	if strings.TrimSpace(msg) == "" {
		return fmt.Sprintf("request_id: %s", requestID)
	}
	return fmt.Sprintf("%s (request_id: %s)", msg, requestID)
}

func replaceStepInCurrentURL(w http.ResponseWriter, r *http.Request) {
	current := strings.TrimSpace(htmx.CurrentUrl(r))
	if current == "" {
		return
	}
	u, err := url.Parse(current)
	if err != nil {
		return
	}
	q := u.Query()
	if q.Get("step") == "" {
		return
	}
	q.Del("step")
	u.RawQuery = q.Encode()
	htmx.ReplaceUrl(w, u.RequestURI())
}

func param(r *http.Request, key string) string {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v != "" {
		return v
	}
	return strings.TrimSpace(r.FormValue(key))
}

func (c *OrgUIController) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	eventType := strings.TrimSpace(r.FormValue("event_type"))
	pernr := strings.TrimSpace(r.FormValue("pernr"))
	orgNodeRaw := strings.TrimSpace(r.FormValue("org_node_id"))
	positionRaw := strings.TrimSpace(r.FormValue("position_id"))

	var orgNodeID *uuid.UUID
	fieldErrs := map[string]string{}
	if eventType == "" {
		fieldErrs["event_type"] = "required"
	} else if eventType != "hire" {
		fieldErrs["event_type"] = "invalid"
	}
	if pernr == "" {
		fieldErrs["pernr"] = "required"
	}
	if orgNodeRaw != "" {
		parsed, err := uuid.Parse(orgNodeRaw)
		if err != nil {
			fieldErrs["org_node_id"] = "invalid uuid"
		} else {
			orgNodeID = &parsed
		}
	}

	var positionID *uuid.UUID
	if positionRaw != "" {
		parsed, err := uuid.Parse(positionRaw)
		if err != nil {
			fieldErrs["position_id"] = "invalid uuid"
		} else {
			positionID = &parsed
		}
	}
	if orgNodeID == nil {
		fieldErrs["org_node_id"] = "required"
	}
	if len(fieldErrs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormCreate,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      eventType,
			Pernr:          pernr,
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         fieldErrs,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.HirePersonnelEvent(r.Context(), tenantID, requestID, initiatorID, services.HirePersonnelEventInput{
		Pernr:         pernr,
		OrgNodeID:     *orgNodeID,
		PositionID:    positionID,
		EffectiveDate: effectiveDate,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)
		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormCreate,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      eventType,
			Pernr:          pernr,
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         map[string]string{},
			FormError:      formErr,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	if includeSummary {
		replaceStepInCurrentURL(w, r)
	} else {
		htmx.PushUrl(w, fmt.Sprintf("/org/assignments?effective_date=%s&pernr=%s", effectiveDateStr, pernr))
	}
	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)
	c.hydrateAssignmentsTimelineLabels(r, tenantID, timeline, effectiveDate)
	c.writeAssignmentsFormWithOOBTimeline(w, r, tenantID, effectiveDateStr, pernr, timeline)
}

func (c *OrgUIController) AssignmentForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	pernr := strings.TrimSpace(r.URL.Query().Get("pernr"))
	if pernr == "" {
		pernr = strings.TrimSpace(r.FormValue("pernr"))
	}
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	eventType := strings.TrimSpace(param(r, "event_type"))
	if eventType == "" {
		eventType = "hire"
	}

	orgNodeIDStr := strings.TrimSpace(param(r, "org_node_id"))
	orgNodeLabel := ""
	if orgNodeIDStr != "" {
		if parsed, err := uuid.Parse(orgNodeIDStr); err == nil {
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, parsed, effectiveDate)
		}
	}
	positionIDStr := strings.TrimSpace(param(r, "position_id"))
	positionLabel := ""
	if positionIDStr != "" {
		if parsed, err := uuid.Parse(positionIDStr); err == nil {
			positionLabel = c.positionLabelFor(r, tenantID, parsed, effectiveDate, "")
		}
	}

	templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
		Mode:           orgui.AssignmentFormCreate,
		EffectiveDate:  effectiveDateStr,
		FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
		EventType:      eventType,
		Pernr:          pernr,
		OrgNodeID:      orgNodeIDStr,
		OrgNodeLabel:   orgNodeLabel,
		PositionID:     positionIDStr,
		PositionLabel:  positionLabel,
		IncludeSummary: includeSummary,
		Errors:         map[string]string{},
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) TransitionAssignmentForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	pernr := strings.TrimSpace(r.URL.Query().Get("pernr"))
	if pernr == "" {
		http.Error(w, "pernr is required", http.StatusBadRequest)
		return
	}

	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)

	var selected *viewmodels.OrgAssignmentRow
	if timeline != nil {
		for i := range timeline.Rows {
			if timeline.Rows[i].ID == assignmentID {
				selected = &timeline.Rows[i]
				break
			}
		}
	}
	if selected == nil {
		http.NotFound(w, r)
		return
	}

	orgNodeID := selected.OrgNodeID
	orgNodeLabel := selected.OrgNodeID.String()
	if details, err := c.getNodeDetails(r, tenantID, selected.OrgNodeID, effectiveDate); err == nil && details != nil {
		if strings.TrimSpace(details.Code) != "" {
			orgNodeLabel = fmt.Sprintf("%s (%s)", strings.TrimSpace(details.Name), strings.TrimSpace(details.Code))
		} else if strings.TrimSpace(details.Name) != "" {
			orgNodeLabel = strings.TrimSpace(details.Name)
		}
	}
	if raw := strings.TrimSpace(param(r, "org_node_id")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			orgNodeID = parsed
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, parsed, effectiveDate)
		}
	}

	positionID := selected.PositionID
	positionLabel := c.positionLabelFor(r, tenantID, selected.PositionID, effectiveDate, strings.TrimSpace(selected.PositionCode))
	if raw := strings.TrimSpace(param(r, "position_id")); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			positionID = parsed
			positionLabel = c.positionLabelFor(r, tenantID, parsed, effectiveDate, "")
		}
	}

	eventType := strings.TrimSpace(param(r, "event_type"))
	if eventType == "" {
		eventType = "transfer"
	}
	templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
		Mode:           orgui.AssignmentFormTransition,
		EffectiveDate:  effectiveDateStr,
		FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
		EventType:      eventType,
		Pernr:          strings.TrimSpace(selected.Pernr),
		AssignmentID:   selected.ID.String(),
		OrgNodeID:      orgNodeID.String(),
		OrgNodeLabel:   orgNodeLabel,
		PositionID:     positionID.String(),
		PositionLabel:  positionLabel,
		IncludeSummary: includeSummary,
		Errors:         map[string]string{},
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) TransitionAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	eventType := strings.TrimSpace(r.FormValue("event_type"))
	pernr := strings.TrimSpace(r.FormValue("pernr"))

	orgNodeRaw := strings.TrimSpace(r.FormValue("org_node_id"))
	positionRaw := strings.TrimSpace(r.FormValue("position_id"))
	reasonNote := strings.TrimSpace(r.FormValue("reason_note"))

	var orgNodeID *uuid.UUID
	fieldErrs := map[string]string{}
	if eventType == "" {
		fieldErrs["event_type"] = "required"
	} else if eventType != "transfer" && eventType != "termination" {
		fieldErrs["event_type"] = "invalid"
	}
	if pernr == "" {
		fieldErrs["pernr"] = "required"
	}
	if strings.TrimSpace(eventType) == "transfer" && orgNodeRaw == "" {
		fieldErrs["org_node_id"] = "required"
	}
	if orgNodeRaw != "" {
		parsed, err := uuid.Parse(orgNodeRaw)
		if err != nil {
			fieldErrs["org_node_id"] = "invalid uuid"
		} else {
			orgNodeID = &parsed
		}
	}

	var positionID *uuid.UUID
	if positionRaw != "" {
		parsed, err := uuid.Parse(positionRaw)
		if err != nil {
			fieldErrs["position_id"] = "invalid uuid"
		} else {
			positionID = &parsed
		}
	}
	if len(fieldErrs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)

		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormTransition,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      eventType,
			Pernr:          pernr,
			AssignmentID:   assignmentID.String(),
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         fieldErrs,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	var reasonNotePtr *string
	if reasonNote != "" {
		reasonNotePtr = &reasonNote
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.TransitionAssignment(r.Context(), tenantID, requestID, initiatorID, services.TransitionAssignmentInput{
		AssignmentID:  assignmentID,
		EventType:     eventType,
		EffectiveDate: effectiveDate,
		OrgNodeID:     orgNodeID,
		PositionID:    positionID,
		ReasonNote:    reasonNotePtr,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)

		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormTransition,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      eventType,
			Pernr:          pernr,
			AssignmentID:   assignmentID.String(),
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         map[string]string{},
			FormError:      formErr,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	if includeSummary {
		replaceStepInCurrentURL(w, r)
	} else {
		htmx.PushUrl(w, fmt.Sprintf("/org/assignments?effective_date=%s&pernr=%s", effectiveDateStr, pernr))
	}
	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)
	c.hydrateAssignmentsTimelineLabels(r, tenantID, timeline, effectiveDate)
	c.writeAssignmentsFormWithOOBTimeline(w, r, tenantID, effectiveDateStr, pernr, timeline)
}

func (c *OrgUIController) EditAssignmentForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	pernr := strings.TrimSpace(r.URL.Query().Get("pernr"))
	if pernr == "" {
		http.Error(w, "pernr is required", http.StatusBadRequest)
		return
	}

	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)

	var selected *viewmodels.OrgAssignmentRow
	if timeline != nil {
		for i := range timeline.Rows {
			if timeline.Rows[i].ID == assignmentID {
				selected = &timeline.Rows[i]
				break
			}
		}
	}
	if selected == nil {
		http.NotFound(w, r)
		return
	}

	orgNodeLabel := selected.OrgNodeID.String()
	if details, err := c.getNodeDetails(r, tenantID, selected.OrgNodeID, effectiveDate); err == nil && details != nil {
		if strings.TrimSpace(details.Code) != "" {
			orgNodeLabel = fmt.Sprintf("%s (%s)", strings.TrimSpace(details.Name), strings.TrimSpace(details.Code))
		} else if strings.TrimSpace(details.Name) != "" {
			orgNodeLabel = strings.TrimSpace(details.Name)
		}
	}
	positionLabel := c.positionLabelFor(r, tenantID, selected.PositionID, effectiveDate, strings.TrimSpace(selected.PositionCode))

	templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
		Mode:           orgui.AssignmentFormEdit,
		EffectiveDate:  effectiveDateStr,
		FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
		EventType:      strings.TrimSpace(param(r, "event_type")),
		Pernr:          strings.TrimSpace(selected.Pernr),
		AssignmentID:   selected.ID.String(),
		OrgNodeID:      selected.OrgNodeID.String(),
		OrgNodeLabel:   orgNodeLabel,
		PositionID:     selected.PositionID.String(),
		PositionLabel:  positionLabel,
		IncludeSummary: includeSummary,
		Errors:         map[string]string{},
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) UpdateAssignment(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgAssignmentsAuthzObject, "assign")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgAssignmentsAuthzObject, "assign") {
		return
	}

	assignmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")
	includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"

	pernr := strings.TrimSpace(r.FormValue("pernr"))
	orgNodeRaw := strings.TrimSpace(r.FormValue("org_node_id"))
	positionRaw := strings.TrimSpace(r.FormValue("position_id"))

	var orgNodeID *uuid.UUID
	fieldErrs := map[string]string{}
	if pernr == "" {
		fieldErrs["pernr"] = "required"
	}
	if orgNodeRaw != "" {
		parsed, err := uuid.Parse(orgNodeRaw)
		if err != nil {
			fieldErrs["org_node_id"] = "invalid uuid"
		} else {
			orgNodeID = &parsed
		}
	}
	var positionID *uuid.UUID
	if positionRaw != "" {
		parsed, err := uuid.Parse(positionRaw)
		if err != nil {
			fieldErrs["position_id"] = "invalid uuid"
		} else {
			positionID = &parsed
		}
	}
	if orgNodeID == nil {
		fieldErrs["org_node_id"] = "required"
	}
	if len(fieldErrs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormEdit,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      strings.TrimSpace(param(r, "event_type")),
			Pernr:          pernr,
			AssignmentID:   assignmentID.String(),
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         fieldErrs,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.UpdateAssignment(r.Context(), tenantID, requestID, initiatorID, services.UpdateAssignmentInput{
		AssignmentID:  assignmentID,
		EffectiveDate: effectiveDate,
		PositionID:    positionID,
		OrgNodeID:     orgNodeID,
	})
	if err != nil {
		formErr, _, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)
		var orgNodeIDStr, orgNodeLabel string
		if orgNodeID != nil {
			orgNodeIDStr = orgNodeID.String()
			orgNodeLabel = c.orgNodeLabelFor(r, tenantID, *orgNodeID, effectiveDate)
		}
		positionLabel := ""
		if positionID != nil {
			positionLabel = c.positionLabelFor(r, tenantID, *positionID, effectiveDate, "")
		}
		templ.Handler(orgui.AssignmentForm(orgui.AssignmentFormProps{
			Mode:           orgui.AssignmentFormEdit,
			EffectiveDate:  effectiveDateStr,
			FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
			EventType:      strings.TrimSpace(param(r, "event_type")),
			Pernr:          pernr,
			AssignmentID:   assignmentID.String(),
			OrgNodeID:      orgNodeIDStr,
			OrgNodeLabel:   orgNodeLabel,
			PositionID:     positionRaw,
			PositionLabel:  positionLabel,
			IncludeSummary: includeSummary,
			Errors:         map[string]string{},
			FormError:      formErr,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	if includeSummary {
		replaceStepInCurrentURL(w, r)
	} else {
		htmx.PushUrl(w, fmt.Sprintf("/org/assignments?effective_date=%s&pernr=%s", effectiveDateStr, pernr))
	}
	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)
	c.hydrateAssignmentsTimelineLabels(r, tenantID, timeline, effectiveDate)
	c.writeAssignmentsFormWithOOBTimeline(w, r, tenantID, effectiveDateStr, pernr, timeline)
}

func (c *OrgUIController) writeAssignmentsFormWithOOBTimeline(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, effectiveDateStr string, pernr string, timeline *viewmodels.OrgAssignmentsTimeline) {
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")

	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgtemplates.AssignmentsHeader(orgtemplates.AssignmentsHeaderProps{
			EffectiveDate: effectiveDateStr,
			Pernr:         pernr,
			SwapOOB:       true,
		}).Render(ctx, ww); err != nil {
			return err
		}
		includeSummary := strings.TrimSpace(param(r, "include_summary")) == "1"
		if includeSummary {
			if _, err := io.WriteString(ww, `<div id="org-assignment-form" class="mt-3"></div>`); err != nil {
				return err
			}
		} else {
			if err := orgui.AssignmentForm(orgui.AssignmentFormProps{
				Mode:           orgui.AssignmentFormCreate,
				EffectiveDate:  effectiveDateStr,
				FreezeCutoff:   c.freezeCutoffFor(r, tenantID),
				EventType:      "hire",
				Pernr:          pernr,
				PositionLabel:  "",
				IncludeSummary: includeSummary,
				Errors:         map[string]string{},
			}).Render(ctx, ww); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(ww, `<div id="org-assignments-timeline" class="p-4" hx-swap-oob="true">`); err != nil {
			return err
		}
		if err := orgui.AssignmentsTimeline(orgui.AssignmentsTimelineProps{EffectiveDate: effectiveDateStr, Pernr: pernr, Timeline: timeline, SwapSummary: includeSummary}).Render(ctx, ww); err != nil {
			return err
		}
		_, err := io.WriteString(ww, `</div>`)
		return err
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) NodeSearchOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgHierarchiesAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type opt struct {
		Value string
		Label string
	}
	out := make([]opt, 0, 50)
	for _, n := range nodes {
		if q != "" && !strings.Contains(strings.ToLower(n.Code), q) && !strings.Contains(strings.ToLower(n.Name), q) {
			continue
		}
		label := strings.TrimSpace(n.Name)
		if strings.TrimSpace(n.Code) != "" {
			label = fmt.Sprintf("%s (%s)", label, strings.TrimSpace(n.Code))
		}
		out = append(out, opt{Value: n.ID.String(), Label: label})
		if len(out) >= 50 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })

	options := make([]*base.ComboboxOption, 0, len(out))
	for _, o := range out {
		options = append(options, &base.ComboboxOption{Value: o.Value, Label: o.Label})
	}
	templ.Handler(orgui.NodeSearchOptions(options), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) NewNodeForm(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgNodesAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("effective_date is required"))
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	var parentID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("parent_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			parentID = &parsed
		}
	}
	parentLabel := ""
	if parentID != nil && *parentID != uuid.Nil {
		parentLabel = c.orgNodeLabelFor(r, tenantID, *parentID, effectiveDate)
	}

	templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
		Mode:                 orgui.NodeFormCreate,
		EffectiveDate:        effectiveDateStr,
		ParentID:             parentID,
		ParentLabel:          parentLabel,
		Errors:               map[string]string{},
		SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
	}), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) NodePanel(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgHierarchiesAuthzObject, "read")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgHierarchiesAuthzObject, "read") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromQuery(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	view := strings.TrimSpace(r.URL.Query().Get("view"))
	switch view {
	case "edit":
		if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
			return
		}
		details, err := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormEdit,
			EffectiveDate:        effectiveDateStr,
			Node:                 details,
			Errors:               map[string]string{},
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	case "move":
		if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgEdgesAuthzObject, "write") {
			return
		}
		details, err := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormMove,
			EffectiveDate:        effectiveDateStr,
			Node:                 details,
			Errors:               map[string]string{},
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	default:
		ensureOrgPageCapabilities(r, orgNodesAuthzObject, "write")
		ensureOrgPageCapabilities(r, orgEdgesAuthzObject, "write")
		c.writeNodePanelWithOOBTree(w, r, tenantID, nodeID, effectiveDateStr, effectiveDate)
		return
	}
}

func (c *OrgUIController) CreateNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgNodesAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	code := strings.TrimSpace(r.FormValue("code"))
	name := strings.TrimSpace(r.FormValue("name"))
	status := strings.TrimSpace(r.FormValue("status"))
	displayOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))

	var parentID *uuid.UUID
	if v := strings.TrimSpace(r.FormValue("parent_id")); v != "" {
		if parsed, err := uuid.Parse(v); err == nil {
			parentID = &parsed
		}
	}
	parentLabel := ""
	if parentID != nil && *parentID != uuid.Nil {
		parentLabel = c.orgNodeLabelFor(r, tenantID, *parentID, effectiveDate)
	}

	i18nNames, i18nErr := parseI18nNames(strings.TrimSpace(r.FormValue("i18n_names")))
	if i18nErr != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormCreate,
			EffectiveDate:        effectiveDateStr,
			Node:                 &viewmodels.OrgNodeDetails{Code: code, Name: name, Status: status, DisplayOrder: displayOrder, I18nNamesJSON: strings.TrimSpace(r.FormValue("i18n_names"))},
			ParentID:             parentID,
			ParentLabel:          parentLabel,
			Errors:               map[string]string{"i18n_names": i18nErr.Error()},
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	res, err := c.org.CreateNode(r.Context(), tenantID, requestID, initiatorID, services.CreateNodeInput{
		Code:          code,
		Name:          name,
		ParentID:      parentID,
		EffectiveDate: effectiveDate,
		I18nNames:     i18nNames,
		Status:        status,
		DisplayOrder:  displayOrder,
	})
	if err != nil {
		formErr, fieldErrs, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormCreate,
			EffectiveDate:        effectiveDateStr,
			Node:                 &viewmodels.OrgNodeDetails{Code: code, Name: name, Status: status, DisplayOrder: displayOrder, I18nNamesJSON: strings.TrimSpace(r.FormValue("i18n_names"))},
			ParentID:             parentID,
			ParentLabel:          parentLabel,
			Errors:               fieldErrs,
			FormError:            formErr,
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	htmx.PushUrl(w, fmt.Sprintf("/org/nodes?effective_date=%s&node_id=%s", effectiveDateStr, res.NodeID.String()))
	c.writeNodePanelWithOOBTree(w, r, tenantID, res.NodeID, effectiveDateStr, effectiveDate)
}

func (c *OrgUIController) UpdateNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgNodesAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	name := strings.TrimSpace(r.FormValue("name"))
	status := strings.TrimSpace(r.FormValue("status"))
	displayOrder, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("display_order")))

	i18nNames, i18nErr := parseI18nNames(strings.TrimSpace(r.FormValue("i18n_names")))
	if i18nErr != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormEdit,
			EffectiveDate:        effectiveDateStr,
			Node:                 &viewmodels.OrgNodeDetails{ID: nodeID, Name: name, Status: status, DisplayOrder: displayOrder, I18nNamesJSON: strings.TrimSpace(r.FormValue("i18n_names"))},
			Errors:               map[string]string{"i18n_names": i18nErr.Error()},
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.UpdateNode(r.Context(), tenantID, requestID, initiatorID, services.UpdateNodeInput{
		NodeID:        nodeID,
		EffectiveDate: effectiveDate,
		Name:          &name,
		I18nNames:     i18nNames,
		Status:        &status,
		DisplayOrder:  &displayOrder,
	})
	if err != nil {
		formErr, fieldErrs, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)
		details, _ := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormEdit,
			EffectiveDate:        effectiveDateStr,
			Node:                 details,
			Errors:               fieldErrs,
			FormError:            formErr,
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	htmx.PushUrl(w, fmt.Sprintf("/org/nodes?effective_date=%s&node_id=%s", effectiveDateStr, nodeID.String()))
	c.writeNodePanelWithOOBTree(w, r, tenantID, nodeID, effectiveDateStr, effectiveDate)
}

func (c *OrgUIController) MoveNode(w http.ResponseWriter, r *http.Request) {
	tenantID, currentUser, ok := tenantAndUserFromContext(r)
	if !ok {
		layouts.WriteAuthzForbiddenResponse(w, r, orgEdgesAuthzObject, "write")
		return
	}
	if !ensureOrgRolloutEnabled(w, r, tenantID) {
		return
	}
	if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgEdgesAuthzObject, "write") {
		return
	}

	nodeID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveDate, err := effectiveDateFromWriteForm(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	newParentRaw := strings.TrimSpace(r.FormValue("new_parent_id"))
	newParentID, err := uuid.Parse(newParentRaw)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		details, _ := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormMove,
			EffectiveDate:        effectiveDateStr,
			Node:                 details,
			Errors:               map[string]string{"new_parent_id": "invalid new_parent_id"},
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.MoveNode(r.Context(), tenantID, requestID, initiatorID, services.MoveNodeInput{
		NodeID:        nodeID,
		NewParentID:   newParentID,
		EffectiveDate: effectiveDate,
	})
	if err != nil {
		formErr, fieldErrs, statusCode := mapServiceErrorToForm(err)
		w.WriteHeader(statusCode)
		details, _ := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormMove,
			EffectiveDate:        effectiveDateStr,
			Node:                 details,
			Errors:               fieldErrs,
			FormError:            formErr,
			SearchParentEndpoint: fmt.Sprintf("/org/nodes/search?effective_date=%s", effectiveDateStr),
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	htmx.PushUrl(w, fmt.Sprintf("/org/nodes?effective_date=%s&node_id=%s", effectiveDateStr, nodeID.String()))
	c.writeNodePanelWithOOBTree(w, r, tenantID, nodeID, effectiveDateStr, effectiveDate)
}

func (c *OrgUIController) writeNodePanelWithOOBTree(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, nodeID uuid.UUID, effectiveDateStr string, effectiveDate time.Time) {
	ensureOrgPageCapabilities(r, orgAssignmentsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgPositionsAuthzObject, "read")
	ensureOrgPageCapabilities(r, orgNodesAuthzObject, "write")
	ensureOrgPageCapabilities(r, orgEdgesAuthzObject, "write")

	details, err := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if details != nil {
		details.LongName = c.orgNodeLongNameFor(r, tenantID, nodeID, effectiveDate)
	}
	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	parentLabelByID := hierarchyNodeLabelMap(nodes)
	parentIDByNodeID := hierarchyParentIDMap(nodes)
	if details != nil {
		if parentID, ok := parentIDByNodeID[nodeID]; ok && parentID != uuid.Nil {
			if label, ok := parentLabelByID[parentID]; ok && strings.TrimSpace(label) != "" {
				details.ParentLabel = label
			} else {
				details.ParentLabel = c.orgNodeLabelFor(r, tenantID, parentID, effectiveDate)
			}
		} else if details.ParentHint != nil && *details.ParentHint != uuid.Nil {
			if label, ok := parentLabelByID[*details.ParentHint]; ok && strings.TrimSpace(label) != "" {
				details.ParentLabel = label
			} else {
				details.ParentLabel = c.orgNodeLabelFor(r, tenantID, *details.ParentHint, effectiveDate)
			}
		}
	}
	selected := nodeID
	tree := mappers.HierarchyToTree(nodes, &selected)
	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
		if err := orgtemplates.NodesHeader(orgtemplates.NodesHeaderProps{
			EffectiveDate: effectiveDateStr,
			SwapOOB:       true,
		}).Render(ctx, ww); err != nil {
			return err
		}
		if err := orgui.NodeDetails(orgui.NodeDetailsProps{EffectiveDate: effectiveDateStr, Node: details}).Render(ctx, ww); err != nil {
			return err
		}
		return orgui.Tree(orgui.TreeProps{Tree: tree, EffectiveDate: effectiveDateStr, SwapOOB: true}).Render(ctx, ww)
	})
	templ.Handler(component, templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *OrgUIController) getNodeDetails(r *http.Request, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) (*viewmodels.OrgNodeDetails, error) {
	node, err := c.org.GetNodeAsOf(r.Context(), tenantID, nodeID, asOf)
	if err != nil {
		return nil, err
	}
	return mappers.NodeDetailsToViewModel(node), nil
}

func (c *OrgUIController) orgNodeLongNameFor(r *http.Request, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) string {
	if nodeID == uuid.Nil {
		return ""
	}

	longNames, err := orglabels.ResolveOrgNodeLongNamesAsOf(r.Context(), tenantID, asOf, []uuid.UUID{nodeID})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(longNames[nodeID])
}

func (c *OrgUIController) orgNodeLabelFor(r *http.Request, tenantID uuid.UUID, nodeID uuid.UUID, asOf time.Time) string {
	label := nodeID.String()
	details, err := c.getNodeDetails(r, tenantID, nodeID, asOf)
	if err != nil || details == nil {
		return label
	}
	if strings.TrimSpace(details.Code) != "" {
		return fmt.Sprintf("%s (%s)", strings.TrimSpace(details.Name), strings.TrimSpace(details.Code))
	}
	if strings.TrimSpace(details.Name) != "" {
		return strings.TrimSpace(details.Name)
	}
	return label
}

func hierarchyNodeLabelMap(nodes []services.HierarchyNode) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string, len(nodes))
	for _, n := range nodes {
		label := strings.TrimSpace(n.Name)
		code := strings.TrimSpace(n.Code)
		if label != "" && code != "" {
			label = fmt.Sprintf("%s (%s)", label, code)
		} else if label == "" && code != "" {
			label = code
		} else if label == "" {
			label = n.ID.String()
		}
		out[n.ID] = label
	}
	return out
}

func hierarchyParentIDMap(nodes []services.HierarchyNode) map[uuid.UUID]uuid.UUID {
	out := make(map[uuid.UUID]uuid.UUID, len(nodes))
	for _, n := range nodes {
		if n.ParentID == nil || *n.ParentID == uuid.Nil {
			continue
		}
		out[n.ID] = *n.ParentID
	}
	return out
}

func (c *OrgUIController) positionLabelFor(r *http.Request, tenantID uuid.UUID, positionID uuid.UUID, asOf time.Time, fallbackCode string) string {
	fallbackCode = strings.TrimSpace(fallbackCode)
	if positionID == uuid.Nil {
		return fallbackCode
	}

	row, _, err := c.org.GetPosition(r.Context(), tenantID, positionID, &asOf)
	if err != nil {
		if fallbackCode != "" {
			return fallbackCode
		}
		return positionID.String()
	}
	label := strings.TrimSpace(row.Code)
	if label == "" {
		label = fallbackCode
	}
	if row.Title != nil && strings.TrimSpace(*row.Title) != "" {
		if label != "" {
			return fmt.Sprintf("%s — %s", label, strings.TrimSpace(*row.Title))
		}
		return strings.TrimSpace(*row.Title)
	}
	return label
}

func (c *OrgUIController) hydrateAssignmentsTimelineLabels(r *http.Request, tenantID uuid.UUID, timeline *viewmodels.OrgAssignmentsTimeline, pageAsOf time.Time) {
	if timeline == nil || len(timeline.Rows) == 0 {
		return
	}

	pageAsOf = normalizeValidTimeDayUTC(pageAsOf)
	if pageAsOf.IsZero() {
		pageAsOf = normalizeValidTimeDayUTC(time.Now().UTC())
	}

	labelAsOfDays := make([]time.Time, len(timeline.Rows))
	queries := make([]orglabels.OrgNodeLongNameQuery, 0, len(timeline.Rows))

	for i := range timeline.Rows {
		rowStart := normalizeValidTimeDayUTC(timeline.Rows[i].EffectiveDate)
		if rowStart.IsZero() {
			continue
		}

		rowEndDay := normalizeValidTimeDayUTC(timeline.Rows[i].EndDate)
		if rowEndDay.IsZero() {
			continue
		}

		labelAsOfDay := labelAsOfDayForAssignmentRow(pageAsOf, rowStart, rowEndDay)
		labelAsOfDays[i] = labelAsOfDay

		if !labelAsOfDay.Equal(rowStart) {
			timeline.Rows[i].OrgNodeLabel = c.orgNodeLabelFor(r, tenantID, timeline.Rows[i].OrgNodeID, labelAsOfDay)
			timeline.Rows[i].PositionLabel = c.positionLabelFor(r, tenantID, timeline.Rows[i].PositionID, labelAsOfDay, strings.TrimSpace(timeline.Rows[i].PositionCode))
		}

		if timeline.Rows[i].OrgNodeID != uuid.Nil {
			queries = append(queries, orglabels.OrgNodeLongNameQuery{
				OrgNodeID: timeline.Rows[i].OrgNodeID,
				AsOfDay:   labelAsOfDay,
			})
		}
	}

	if len(queries) == 0 {
		return
	}

	longNamesByKey, err := orglabels.ResolveOrgNodeLongNames(r.Context(), tenantID, queries)
	if err != nil {
		return
	}

	for i := range timeline.Rows {
		if timeline.Rows[i].OrgNodeID == uuid.Nil {
			continue
		}
		labelAsOfDay := labelAsOfDays[i]
		if labelAsOfDay.IsZero() {
			continue
		}

		timeline.Rows[i].OrgNodeLongName = strings.TrimSpace(longNamesByKey[orglabels.OrgNodeLongNameKey{
			OrgNodeID: timeline.Rows[i].OrgNodeID,
			AsOfDate:  labelAsOfDay.Format(time.DateOnly),
		}])
	}
}

func labelAsOfDayForAssignmentRow(pageAsOfDay, rowStartDay, rowEndDay time.Time) time.Time {
	pageAsOfDay = normalizeValidTimeDayUTC(pageAsOfDay)
	rowStartDay = normalizeValidTimeDayUTC(rowStartDay)
	rowEndDay = normalizeValidTimeDayUTC(rowEndDay)

	if rowStartDay.IsZero() || rowEndDay.IsZero() {
		return rowStartDay
	}

	if openEndedEndDate(rowEndDay) {
		if !pageAsOfDay.Before(rowStartDay) {
			return pageAsOfDay
		}
		return rowStartDay
	}

	if !pageAsOfDay.Before(rowStartDay) && !pageAsOfDay.After(rowEndDay) {
		return pageAsOfDay
	}
	return rowStartDay
}

func tenantAndUserFromContext(r *http.Request) (uuid.UUID, coreuser.User, bool) {
	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		return uuid.Nil, nil, false
	}
	currentUser, err := composables.UseUser(r.Context())
	if err != nil || currentUser == nil {
		return uuid.Nil, nil, false
	}
	return tenantID, currentUser, true
}

func effectiveDateFromQuery(r *http.Request) (time.Time, error) {
	v := strings.TrimSpace(r.URL.Query().Get("effective_date"))
	t, err := parseEffectiveDate(v)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func effectiveDateFromWriteForm(r *http.Request) (time.Time, error) {
	v := strings.TrimSpace(r.PostFormValue("effective_date"))
	t, err := parseEffectiveDate(v)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func (c *OrgUIController) freezeCutoffFor(r *http.Request, tenantID uuid.UUID) string {
	info, err := c.org.GetFreezeInfo(r.Context(), tenantID, time.Now().UTC())
	if err != nil {
		return ""
	}
	if strings.TrimSpace(info.Mode) == "disabled" {
		return ""
	}
	if info.CutoffUTC.IsZero() {
		return ""
	}
	return info.CutoffUTC.UTC().Format("2006-01-02")
}

func parseI18nNames(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("invalid json")
	}
	return out, nil
}

func mapServiceErrorToForm(err error) (string, map[string]string, int) {
	var svcErr *services.ServiceError
	if errors.As(err, &svcErr) {
		code := svcErr.Code
		fieldErrs := map[string]string{}
		switch code {
		case "ORG_INVALID_BODY":
			return svcErr.Message, fieldErrs, http.StatusUnprocessableEntity
		case "ORG_OVERLAP", "ORG_USE_CORRECT", "ORG_USE_CORRECT_MOVE":
			return svcErr.Message, fieldErrs, http.StatusConflict
		default:
			if svcErr.Status == http.StatusUnprocessableEntity {
				return svcErr.Message, fieldErrs, http.StatusUnprocessableEntity
			}
			if svcErr.Status == http.StatusConflict {
				return svcErr.Message, fieldErrs, http.StatusConflict
			}
			return svcErr.Message, fieldErrs, http.StatusBadRequest
		}
	}
	return err.Error(), map[string]string{}, http.StatusBadRequest
}

func ensureOrgAuthzUI(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, currentUser coreuser.User, object, action string) bool {
	capKey := authzutil.CapabilityKey(object, action)

	ctxWithState, _ := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}
	svc := authz.Use()
	mode := svc.Mode()
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authzutil.DomainFromContext(r.Context()),
		object,
		authz.NormalizeAction(action),
	)

	allowed, err := enforceOrgRequest(r.Context(), svc, req, mode)
	state := authz.ViewStateFromContext(r.Context())
	if err != nil {
		recordOrgForbiddenCapability(state, authzutil.DomainFromContext(r.Context()), object, action, capKey)
		layouts.WriteAuthzForbiddenResponse(w, r, object, action)
		return false
	}
	if allowed {
		if state != nil {
			state.SetCapability(capKey, true)
		}
		return true
	}

	recordOrgForbiddenCapability(state, authzutil.DomainFromContext(r.Context()), object, action, capKey)
	layouts.WriteAuthzForbiddenResponse(w, r, object, action)
	return false
}

func enforceOrgRequest(ctx context.Context, svc *authz.Service, req authz.Request, mode authz.Mode) (bool, error) {
	if svc == nil {
		return true, nil
	}
	if err := svc.Authorize(ctx, req); err != nil {
		return false, err
	}
	switch mode {
	case authz.ModeDisabled, authz.ModeEnforce:
		return true, nil
	case authz.ModeShadow:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	default:
		allowed, err := svc.Check(ctx, req)
		if err != nil {
			return false, err
		}
		return allowed, nil
	}
}

func recordOrgForbiddenCapability(state *authz.ViewState, domain, object, action, capKey string) {
	if state == nil {
		return
	}
	state.SetCapability(capKey, false)
	state.AddMissingPolicy(authz.MissingPolicy{
		Domain: domain,
		Object: object,
		Action: authz.NormalizeAction(action),
	})
}

func ensureOrgPageCapabilities(r *http.Request, object string, actions ...string) {
	if len(actions) == 0 || strings.TrimSpace(object) == "" {
		return
	}
	state := authz.ViewStateFromContext(r.Context())
	if state == nil {
		return
	}
	currentUser, err := composables.UseUser(r.Context())
	if err != nil || currentUser == nil {
		return
	}
	tenantID := authzutil.TenantIDFromContext(r.Context())
	logger := composables.UseLogger(r.Context())
	for _, action := range actions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		if _, _, err := authzutil.CheckCapability(r.Context(), state, tenantID, currentUser, object, action); err != nil {
			logger.WithError(err).WithField("capability", action).Warn("failed to evaluate capability")
		}
	}
}
