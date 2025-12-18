package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	router.HandleFunc("/assignments", c.CreateAssignment).Methods(http.MethodPost)
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
	ensureOrgPageCapabilities(r, orgNodesAuthzObject, "write")
	ensureOrgPageCapabilities(r, orgEdgesAuthzObject, "write")

	effectiveDate, err := effectiveDateFromRequest(r)
	if err != nil {
		http.Error(w, "invalid effective_date", http.StatusBadRequest)
		return
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

	var errs []string
	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		errs = append(errs, err.Error())
	}
	tree := mappers.HierarchyToTree(nodes, selectedNodeID)

	var selected *viewmodels.OrgNodeDetails
	if selectedNodeID != nil {
		details, err := c.getNodeDetails(r, tenantID, *selectedNodeID, effectiveDate)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			selected = details
		}
	}

	props := orgtemplates.NodesPageProps{
		EffectiveDate: effectiveDateStr,
		Tree:          tree,
		SelectedNode:  selected,
		Errors:        errs,
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

	effectiveDate, err := effectiveDateFromRequest(r)
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

	effectiveDate, err := effectiveDateFromRequest(r)
	if err != nil {
		http.Error(w, "invalid effective_date", http.StatusBadRequest)
		return
	}
	if effectiveDate.IsZero() {
		effectiveDate = time.Now().UTC()
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
		_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, &effectiveDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		timeline = mappers.AssignmentsToTimeline(subject, rows)
	}

	if htmx.IsHxRequest(r) {
		templ.Handler(orgui.AssignmentsTimeline(orgui.AssignmentsTimelineProps{
			EffectiveDate: effectiveDateStr,
			Timeline:      timeline,
		}), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	props := orgtemplates.AssignmentsPageProps{
		EffectiveDate: effectiveDateStr,
		Pernr:         pernr,
		Timeline:      timeline,
	}
	templ.Handler(orgtemplates.AssignmentsPage(props), templ.WithStreaming()).ServeHTTP(w, r)
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

	effectiveDate, err := effectiveDateFromRequest(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	pernr := strings.TrimSpace(r.FormValue("pernr"))
	orgNodeRaw := strings.TrimSpace(r.FormValue("org_node_id"))
	positionRaw := strings.TrimSpace(r.FormValue("position_id"))

	var orgNodeID *uuid.UUID
	if orgNodeRaw != "" {
		if parsed, err := uuid.Parse(orgNodeRaw); err == nil {
			orgNodeID = &parsed
		}
	}

	var positionID *uuid.UUID
	if positionRaw != "" {
		if parsed, err := uuid.Parse(positionRaw); err == nil {
			positionID = &parsed
		}
	}

	initiatorID := authzutil.NormalizedUserUUID(tenantID, currentUser)
	requestID := ensureRequestID(r)
	_, err = c.org.CreateAssignment(r.Context(), tenantID, requestID, initiatorID, services.CreateAssignmentInput{
		Pernr:          pernr,
		EffectiveDate:  effectiveDate,
		PositionID:     positionID,
		OrgNodeID:      orgNodeID,
		AssignmentType: "primary",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	htmx.PushUrl(w, fmt.Sprintf("/org/assignments?effective_date=%s&pernr=%s", effectiveDateStr, pernr))
	subject := fmt.Sprintf("person:%s", pernr)
	_, rows, _, err := c.org.GetAssignments(r.Context(), tenantID, subject, &effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	timeline := mappers.AssignmentsToTimeline(subject, rows)
	templ.Handler(orgui.AssignmentsTimeline(orgui.AssignmentsTimelineProps{
		EffectiveDate: effectiveDateStr,
		Timeline:      timeline,
	}), templ.WithStreaming()).ServeHTTP(w, r)
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

	effectiveDate, err := effectiveDateFromRequest(r)
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

	effectiveDate, err := effectiveDateFromRequest(r)
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

	templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
		Mode:                 orgui.NodeFormCreate,
		EffectiveDate:        effectiveDateStr,
		ParentID:             parentID,
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

	effectiveDate, err := effectiveDateFromRequest(r)
	if err != nil || effectiveDate.IsZero() {
		http.Error(w, "effective_date is required", http.StatusBadRequest)
		return
	}
	effectiveDateStr := effectiveDate.UTC().Format("2006-01-02")

	details, err := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	view := strings.TrimSpace(r.URL.Query().Get("view"))
	switch view {
	case "edit":
		if !ensureOrgAuthzUI(w, r, tenantID, currentUser, orgNodesAuthzObject, "write") {
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
		templ.Handler(orgui.NodeDetails(orgui.NodeDetailsProps{
			EffectiveDate: effectiveDateStr,
			Node:          details,
		}), templ.WithStreaming()).ServeHTTP(w, r)
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

	effectiveDate, err := effectiveDateFromRequest(r)
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

	i18nNames, i18nErr := parseI18nNames(strings.TrimSpace(r.FormValue("i18n_names")))
	if i18nErr != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		templ.Handler(orgui.NodeForm(orgui.NodeFormProps{
			Mode:                 orgui.NodeFormCreate,
			EffectiveDate:        effectiveDateStr,
			ParentID:             parentID,
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
			ParentID:             parentID,
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

	effectiveDate, err := effectiveDateFromRequest(r)
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
			Node:                 &viewmodels.OrgNodeDetails{ID: nodeID},
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

	effectiveDate, err := effectiveDateFromRequest(r)
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
	details, err := c.getNodeDetails(r, tenantID, nodeID, effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nodes, _, err := c.org.GetHierarchyAsOf(r.Context(), tenantID, "OrgUnit", effectiveDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	selected := nodeID
	tree := mappers.HierarchyToTree(nodes, &selected)
	component := templ.ComponentFunc(func(ctx context.Context, ww io.Writer) error {
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

func effectiveDateFromRequest(r *http.Request) (time.Time, error) {
	v := strings.TrimSpace(r.URL.Query().Get("effective_date"))
	if v == "" {
		v = strings.TrimSpace(r.FormValue("effective_date"))
	}
	t, err := parseEffectiveDate(v)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
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
