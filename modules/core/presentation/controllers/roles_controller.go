package controllers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/role"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/roles"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/di"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/mapping"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
	"github.com/iota-uz/iota-sdk/pkg/rbac"
	"github.com/iota-uz/iota-sdk/pkg/repo"
	"github.com/iota-uz/iota-sdk/pkg/shared"
	"github.com/sirupsen/logrus"

	"github.com/a-h/templ"
	"github.com/gorilla/mux"
)

type RolesController struct {
	app              application.Application
	basePath         string
	permissionSchema *rbac.PermissionSchema
	stageStore       *policyStageStore
}

var rolesAuthzObject = authz.ObjectName("core", "roles")

func ensureRolesAuthz(w http.ResponseWriter, r *http.Request, action string) bool {
	return ensureAuthz(w, r, rolesAuthzObject, action, legacyRolePermission(action))
}

func legacyRolePermission(action string) *permission.Permission {
	switch action {
	case "list", "view":
		return permissions.RoleRead
	case "create":
		return permissions.RoleCreate
	case "update":
		return permissions.RoleUpdate
	case "delete":
		return permissions.RoleDelete
	default:
		return nil
	}
}

type RolesControllerOptions struct {
	BasePath         string
	PermissionSchema *rbac.PermissionSchema
}

func NewRolesController(app application.Application, opts *RolesControllerOptions) application.Controller {
	if opts == nil || opts.PermissionSchema == nil {
		panic("RolesController requires PermissionSchema in options")
	}
	if opts.BasePath == "" {
		panic("RolesController requires explicit BasePath in options")
	}
	return &RolesController{
		app:              app,
		basePath:         opts.BasePath,
		permissionSchema: opts.PermissionSchema,
		stageStore:       usePolicyStageStore(),
	}
}

func (c *RolesController) Key() string {
	return c.basePath
}

func (c *RolesController) Register(r *mux.Router) {
	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	)
	router.HandleFunc("", di.H(c.List)).Methods(http.MethodGet)
	router.HandleFunc("/new", di.H(c.GetNew)).Methods(http.MethodGet)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.GetEdit)).Methods(http.MethodGet)
	router.HandleFunc("/{id:[0-9]+}/policies", di.H(c.GetPolicies)).Methods(http.MethodGet)

	router.HandleFunc("", di.H(c.Create)).Methods(http.MethodPost)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.Update)).Methods(http.MethodPost)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.Delete)).Methods(http.MethodDelete)
}

func (c *RolesController) modulePermissionGroups(
	selected ...*permission.Permission,
) []*viewmodels.ModulePermissionGroup {
	return BuildModulePermissionGroups(c.permissionSchema, selected...)
}

func (c *RolesController) List(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
) {
	if !ensureRolesAuthz(w, r, "list") {
		return
	}

	ensurePageCapabilities(r, rolesAuthzObject, "create", "update", "delete", "view")

	params := composables.UsePaginated(r)
	search := r.URL.Query().Get("name")

	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		logger.Errorf("Error retrieving tenant from request context: %v", err)
		http.Error(w, "Error retrieving tenant", http.StatusBadRequest)
		return
	}

	// Create find params with search
	findParams := &role.FindParams{
		Limit:  params.Limit,
		Offset: params.Offset,
		Filters: []role.Filter{
			{
				Column: role.TenantIDField,
				Filter: repo.Eq(tenantID.String()),
			},
		},
	}

	// Apply search filter if provided
	if search != "" {
		findParams.Search = search
	}

	roleEntities, err := roleService.GetPaginated(r.Context(), findParams)
	if err != nil {
		logger.Errorf("Error retrieving roles: %v", err)
		http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
		return
	}

	props := &roles.IndexPageProps{
		Roles:  mapping.MapViewModels(roleEntities, mappers.RoleToViewModel),
		Search: search,
	}

	if htmx.IsHxRequest(r) {
		templ.Handler(roles.RoleRows(props), templ.WithStreaming()).ServeHTTP(w, r)
	} else {
		templ.Handler(roles.Index(props), templ.WithStreaming()).ServeHTTP(w, r)
	}
}

func (c *RolesController) GetEdit(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
	policyService *services.PolicyDraftService,
) {
	if !ensureRolesAuthz(w, r, "view") {
		return
	}

	ensurePageCapabilities(r, rolesAuthzObject, "update", "delete")
	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing role ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	roleEntity, err := roleService.GetByID(r.Context(), id)
	if err != nil {
		logger.Errorf("Error retrieving role: %v", err)
		http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
		return
	}
	matrixProps, matrixErr := c.buildPolicyMatrixProps(r.Context(), r, policyService, roleEntity, PolicyListParams{})
	if matrixErr != nil {
		logger.WithError(matrixErr).Error("Error building policy matrix")
	}
	props := &roles.EditFormProps{
		Role:                   mappers.RoleToViewModel(roleEntity),
		ModulePermissionGroups: c.modulePermissionGroups(roleEntity.Permissions()...),
		Errors:                 map[string]string{},
		PolicyMatrix:           matrixProps,
	}
	templ.Handler(roles.Edit(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *RolesController) Delete(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
) {
	if !ensureRolesAuthz(w, r, "delete") {
		return
	}
	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing role ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := roleService.Delete(r.Context(), id); err != nil {
		logger.Errorf("Error deleting role: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shared.Redirect(w, r, c.basePath)
}

func (c *RolesController) Update(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
) {
	if !ensureRolesAuthz(w, r, "update") {
		return
	}
	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing role ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dto, err := composables.UseForm(&dtos.UpdateRoleDTO{}, r)
	if err != nil {
		logger.Errorf("Error parsing form: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roleEntity, err := roleService.GetByID(r.Context(), id)
	if err != nil {
		logger.Errorf("Error retrieving role: %v", err)
		http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
		return
	}

	if errors, ok := dto.Ok(r.Context()); !ok {
		props := &roles.EditFormProps{
			Role:                   mappers.RoleToViewModel(roleEntity),
			ModulePermissionGroups: c.modulePermissionGroups(roleEntity.Permissions()...),
			Errors:                 errors,
		}
		templ.Handler(roles.EditForm(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	updatedEntity, err := dto.Apply(roleEntity, c.permissionSchema)
	if err != nil {
		logger.Errorf("Error updating role entity: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := roleService.Update(r.Context(), updatedEntity); err != nil {
		logger.Errorf("Error updating role: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shared.Redirect(w, r, c.basePath)
}

func (c *RolesController) GetNew(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
) {
	if !ensureRolesAuthz(w, r, "create") {
		return
	}

	ensurePageCapabilities(r, rolesAuthzObject, "create")
	props := &roles.CreateFormProps{
		Role:                   &viewmodels.Role{},
		ModulePermissionGroups: c.modulePermissionGroups(),
		Errors:                 map[string]string{},
	}
	templ.Handler(roles.New(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *RolesController) Create(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
) {
	if !ensureRolesAuthz(w, r, "create") {
		return
	}
	dto, err := composables.UseForm(&dtos.CreateRoleDTO{}, r)
	if err != nil {
		logger.Errorf("Error parsing form: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if errors, ok := dto.Ok(r.Context()); !ok {
		roleEntity, err := dto.ToEntity(c.permissionSchema)
		if err != nil {
			logger.Errorf("Error converting DTO to entity: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		props := &roles.CreateFormProps{
			Role:                   mappers.RoleToViewModel(roleEntity),
			ModulePermissionGroups: c.modulePermissionGroups(),
			Errors:                 errors,
		}
		templ.Handler(roles.CreateForm(props), templ.WithStreaming()).ServeHTTP(w, r)
		return
	}

	roleEntity, err := dto.ToEntity(c.permissionSchema)
	if err != nil {
		logger.Errorf("Error converting DTO to entity: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := roleService.Create(r.Context(), roleEntity); err != nil {
		logger.Errorf("Error creating role: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shared.Redirect(w, r, c.basePath)
}

func (c *RolesController) GetPolicies(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
	policyService *services.PolicyDraftService,
) {
	if !ensureRolesAuthz(w, r, "view") {
		return
	}
	if !ensureAuthz(w, r, authz.ObjectName("core", "authz"), "debug", nil) {
		return
	}
	id, err := shared.ParseID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roleEntity, err := roleService.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Error retrieving role", http.StatusInternalServerError)
		return
	}
	params, err := parsePolicyListParams(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "AUTHZ_INVALID_QUERY", err.Error())
		return
	}
	matrixProps, err := c.buildPolicyMatrixProps(r.Context(), r, policyService, roleEntity, params)
	if err != nil {
		logger.WithError(err).Error("failed to build policy matrix")
		writeJSONError(w, http.StatusInternalServerError, "AUTHZ_POLICIES_ERROR", "failed to load policies")
		return
	}
	templ.Handler(roles.PolicyMatrix(matrixProps), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *RolesController) buildPolicyMatrixProps(
	ctx context.Context,
	r *http.Request,
	policyService *services.PolicyDraftService,
	roleEntity role.Role,
	params PolicyListParams,
) (*roles.PolicyMatrixProps, error) {
	entries, err := policyService.Policies(ctx)
	if err != nil {
		return nil, err
	}
	if params.Page == 0 {
		params.Page = 1
	}
	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.Subject == "" {
		params.Subject = authz.SubjectForRole(roleEntity.Name())
	}
	if params.Domain == "" {
		params.Domain = authz.DomainFromTenant(roleEntity.TenantID())
	}
	var stagedEntries []dtos.StagedPolicyEntry
	var canStage bool
	var canRequest bool
	if currentUser, err := composables.UseUser(ctx); err == nil && currentUser != nil {
		if tenantID, tenantErr := composables.UseTenantID(ctx); tenantErr == nil {
			stagedEntries = c.stageStore.List(
				policyStageKey(currentUser.ID(), tenantID),
				params.Subject,
				params.Domain,
			)
		}
	}
	if err := composables.CanUser(ctx, permissions.AuthzRequestsWrite); err == nil {
		canRequest = true
	}
	if canRequest {
		canStage = composables.CanUser(ctx, permissions.RoleUpdate) == nil
	}
	opts := buildAuthzSelectorOptions(entries)
	matrixEntries, total := mergePolicyMatrixEntries(entries, stagedEntries, params)
	return &roles.PolicyMatrixProps{
		RoleID:        fmt.Sprintf("%d", roleEntity.ID()),
		Entries:       matrixEntries,
		Total:         total,
		Page:          params.Page,
		Limit:         params.Limit,
		ObjectOptions: opts.Objects,
		ActionOptions: opts.Actions,
		RoleOptions:   opts.Roles,
		Subject:       params.Subject,
		Domain:        params.Domain,
		Type:          params.Type,
		Search:        params.Search,
		CanDebug:      true,
		CanStage:      canStage,
		CanRequest:    canRequest,
		StageTotal: func() int {
			if stagedEntries == nil {
				return 0
			}
			return len(stagedEntries)
		}(),
		StageSummary: summarizeStagedEntries(stagedEntries),
		StagePreview: buildWorkspacePreview(stagedEntries),
	}, nil
}

func mergePolicyMatrixEntries(
	baseEntries []services.PolicyEntry,
	staged []dtos.StagedPolicyEntry,
	params PolicyListParams,
) ([]roles.PolicyMatrixEntry, int) {
	filtered := filterPolicies(baseEntries, params)

	stagePolicies := make([]services.PolicyEntry, 0, len(staged))
	stageMeta := make(map[string]dtos.StagedPolicyEntry, len(staged))
	for _, entry := range staged {
		policy := services.PolicyEntry{
			Type:    entry.Type,
			Subject: entry.Subject,
			Domain:  entry.Domain,
			Object:  entry.Object,
			Action:  entry.Action,
			Effect:  entry.Effect,
		}
		stageMeta[policyEntryKey(policy)] = entry
		stagePolicies = append(stagePolicies, policy)
	}
	filteredStage := filterPolicies(stagePolicies, params)
	stageOverlay := make(map[string]dtos.StagedPolicyEntry, len(filteredStage))
	for _, entry := range filteredStage {
		key := policyEntryKey(entry)
		if stagedEntry, ok := stageMeta[key]; ok {
			stageOverlay[key] = stagedEntry
		}
	}

	rows := make([]roles.PolicyMatrixEntry, 0, len(filtered)+len(stageOverlay))
	for _, entry := range filtered {
		key := policyEntryKey(entry)
		row := roles.PolicyMatrixEntry{
			PolicyEntryResponse: dtos.PolicyEntryResponse{
				Type:    entry.Type,
				Subject: entry.Subject,
				Domain:  entry.Domain,
				Object:  entry.Object,
				Action:  entry.Action,
				Effect:  entry.Effect,
			},
		}
		if stagedEntry, ok := stageOverlay[key]; ok {
			row.Staged = true
			if stagedEntry.StageKind != "" {
				row.StageKind = stagedEntry.StageKind
			} else {
				row.StageKind = "add"
			}
			row.StageID = stagedEntry.ID
			row.StageOnly = false
			delete(stageOverlay, key)
		}
		rows = append(rows, row)
	}
	for _, stagedEntry := range stageOverlay {
		rows = append(rows, roles.PolicyMatrixEntry{
			PolicyEntryResponse: stagedEntry.PolicyEntryResponse,
			StageID:             stagedEntry.ID,
			StageKind:           stagedEntry.StageKind,
			Staged:              true,
			StageOnly:           true,
		})
	}

	sortPolicyMatrixEntries(rows, params.SortField, params.SortAsc)
	total := len(rows)
	start := params.Offset()
	end := start + params.Limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	return rows[start:end], total
}

func sortPolicyMatrixEntries(entries []roles.PolicyMatrixEntry, field string, asc bool) {
	less := func(i, j int) bool { return entries[i].Object < entries[j].Object }
	switch field {
	case "subject":
		less = func(i, j int) bool { return entries[i].Subject < entries[j].Subject }
	case "domain":
		less = func(i, j int) bool { return entries[i].Domain < entries[j].Domain }
	case "type":
		less = func(i, j int) bool { return entries[i].Type < entries[j].Type }
	case "action":
		less = func(i, j int) bool { return entries[i].Action < entries[j].Action }
	case "effect":
		less = func(i, j int) bool { return entries[i].Effect < entries[j].Effect }
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if asc {
			return less(i, j)
		}
		return less(j, i)
	})
}

func policyEntryKey(entry services.PolicyEntry) string {
	return strings.Join([]string{
		entry.Type,
		entry.Subject,
		entry.Domain,
		entry.Object,
		entry.Action,
		entry.Effect,
	}, "|")
}
