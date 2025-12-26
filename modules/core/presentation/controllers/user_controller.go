package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/components/base"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/role"
	"github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/query"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/users"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/di"
	"github.com/iota-uz/iota-sdk/pkg/htmx"
	"github.com/iota-uz/iota-sdk/pkg/mapping"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
	"github.com/iota-uz/iota-sdk/pkg/rbac"
	"github.com/iota-uz/iota-sdk/pkg/repo"
	"github.com/iota-uz/iota-sdk/pkg/shared"
	"github.com/iota-uz/iota-sdk/pkg/validators"
	"github.com/sirupsen/logrus"
)

type UserRealtimeUpdates struct {
	app         application.Application
	userService *services.UserService
	basePath    string
}

func NewUserRealtimeUpdates(app application.Application, userService *services.UserService, basePath string) *UserRealtimeUpdates {
	return &UserRealtimeUpdates{
		app:         app,
		userService: userService,
		basePath:    basePath,
	}
}

func (ru *UserRealtimeUpdates) Register() {
	ru.app.EventPublisher().Subscribe(ru.onUserCreated)
	ru.app.EventPublisher().Subscribe(ru.onUserUpdated)
	ru.app.EventPublisher().Subscribe(ru.onUserDeleted)
}

func (ru *UserRealtimeUpdates) onUserCreated(event *user.CreatedEvent) {
	logger := configuration.Use().Logger()

	component := users.UserCreatedEvent(mappers.UserToViewModel(event.Result), &base.TableRowProps{
		Attrs: templ.Attributes{},
	})

	if err := ru.app.Websocket().ForEach(application.ChannelAuthenticated, func(connCtx context.Context, conn application.Connection) error {
		var buf bytes.Buffer
		if err := component.Render(connCtx, &buf); err != nil {
			logger.WithError(err).Error("failed to render user created event for websocket")
			return nil // Continue processing other connections
		}
		if err := conn.SendMessage(buf.Bytes()); err != nil {
			logger.WithError(err).Error("failed to send user created event to websocket connection")
			return nil // Continue processing other connections
		}
		return nil
	}); err != nil {
		logger.WithError(err).Error("failed to broadcast user created event to websocket")
		return
	}
}

func (ru *UserRealtimeUpdates) onUserDeleted(event *user.DeletedEvent) {
	logger := configuration.Use().Logger()

	component := users.UserRow(mappers.UserToViewModel(event.Result), &base.TableRowProps{
		Attrs: templ.Attributes{
			"hx-swap-oob": "delete",
		},
	})

	err := ru.app.Websocket().ForEach(application.ChannelAuthenticated, func(connCtx context.Context, conn application.Connection) error {
		var buf bytes.Buffer
		if err := component.Render(connCtx, &buf); err != nil {
			logger.WithError(err).Error("failed to render user deleted event for websocket")
			return nil // Continue processing other connections
		}
		if err := conn.SendMessage(buf.Bytes()); err != nil {
			logger.WithError(err).Error("failed to send user deleted event to websocket connection")
			return nil // Continue processing other connections
		}
		return nil
	})
	if err != nil {
		logger.WithError(err).Error("failed to broadcast user deleted event to websocket")
		return
	}
}

func (ru *UserRealtimeUpdates) onUserUpdated(event *user.UpdatedEvent) {
	logger := configuration.Use().Logger()

	component := users.UserRow(mappers.UserToViewModel(event.Result), &base.TableRowProps{
		Attrs: templ.Attributes{},
	})

	if err := ru.app.Websocket().ForEach(application.ChannelAuthenticated, func(connCtx context.Context, conn application.Connection) error {
		var buf bytes.Buffer
		if err := component.Render(connCtx, &buf); err != nil {
			logger.WithError(err).Error("failed to render user updated event for websocket")
			return nil // Continue processing other connections
		}
		if err := conn.SendMessage(buf.Bytes()); err != nil {
			logger.WithError(err).Error("failed to send user updated event to websocket connection")
			return nil // Continue processing other connections
		}
		return nil
	}); err != nil {
		logger.WithError(err).Error("failed to broadcast user updated event to websocket")
		return
	}
}

type UsersController struct {
	app              application.Application
	basePath         string
	realtime         *UserRealtimeUpdates
	permissionSchema *rbac.PermissionSchema
	stageStore       *policyStageStore
}

var usersAuthzObject = authz.ObjectName("core", "users")

const (
	userPolicyColumnInherited = "inherited"
	userPolicyColumnDirect    = "direct"
	userPolicyColumnOverrides = "overrides"
)

func ensureUsersAuthz(w http.ResponseWriter, r *http.Request, action string) bool {
	return ensureAuthz(w, r, usersAuthzObject, action, legacyUserPermission(action))
}

func legacyUserPermission(action string) *permission.Permission {
	switch action {
	case "list", "view":
		return permissions.UserRead
	case "create":
		return permissions.UserCreate
	case "update":
		return permissions.UserUpdate
	case "delete":
		return permissions.UserDelete
	default:
		return nil
	}
}

type UsersControllerOptions struct {
	BasePath         string
	PermissionSchema *rbac.PermissionSchema
}

func NewUsersController(app application.Application, opts *UsersControllerOptions) application.Controller {
	if opts == nil || opts.PermissionSchema == nil {
		panic("UsersController requires PermissionSchema in options")
	}
	if opts.BasePath == "" {
		panic("UsersController requires explicit BasePath in options")
	}
	userService := app.Service(services.UserService{}).(*services.UserService)

	controller := &UsersController{
		app:              app,
		basePath:         opts.BasePath,
		realtime:         NewUserRealtimeUpdates(app, userService, opts.BasePath),
		permissionSchema: opts.PermissionSchema,
		stageStore:       usePolicyStageStore(),
	}

	return controller
}

func (c *UsersController) Key() string {
	return c.basePath
}

func (c *UsersController) Register(r *mux.Router) {
	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(
		middleware.Authorize(),
		middleware.RedirectNotAuthenticated(),
		middleware.ProvideUser(),
		middleware.ProvideDynamicLogo(c.app),
		middleware.ProvideLocalizer(c.app),
		middleware.NavItems(),
		middleware.WithPageContext(),
	)
	router.HandleFunc("", di.H(c.Users)).Methods(http.MethodGet)
	router.HandleFunc("/new", di.H(c.GetNew)).Methods(http.MethodGet)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.GetEdit)).Methods(http.MethodGet)
	router.HandleFunc("/{id:[0-9]+}/policies", di.H(c.GetPolicies)).Methods(http.MethodGet)

	router.HandleFunc("", di.H(c.Create)).Methods(http.MethodPost)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.Update)).Methods(http.MethodPost)
	router.HandleFunc("/{id:[0-9]+}", di.H(c.Delete)).Methods(http.MethodDelete)

	c.realtime.Register()
}

func (c *UsersController) resourcePermissionGroups(
	selected ...*permission.Permission,
) []*viewmodels.ResourcePermissionGroup {
	return BuildResourcePermissionGroups(c.permissionSchema, selected...)
}

func (c *UsersController) Users(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
	userQueryService *services.UserQueryService,
	groupQueryService *services.GroupQueryService,
	roleQueryService *services.RoleQueryService,
) {
	if !ensureUsersAuthz(w, r, "list") {
		return
	}

	ensurePageCapabilities(r, usersAuthzObject, "create", "update")
	ensurePageCapabilities(r, groupsAuthzObject, "list")
	ensurePageCapabilities(r, rolesAuthzObject, "list")

	pageCtx, _ := composables.TryUsePageCtx(r.Context())

	params := composables.UsePaginated(r)
	groupIDs := r.URL.Query()["groupID"]
	roleIDs := r.URL.Query()["roleID"]

	// Create find params using the query service types
	findParams := &query.FindParams{
		Limit:  params.Limit,
		Offset: params.Offset,
		SortBy: query.SortBy{
			Fields: []repo.SortByField[query.Field]{
				{
					Field:     "created_at",
					Ascending: false,
				},
			},
		},
		Search:  r.URL.Query().Get("Search"),
		Filters: []query.Filter{},
	}

	// Add group filter if specified
	if len(groupIDs) > 0 {
		findParams.Filters = append(findParams.Filters, query.Filter{
			Column: query.FieldGroupID,
			Filter: repo.In(groupIDs),
		})
	}

	// Add role filter if specified
	if len(roleIDs) > 0 {
		findParams.Filters = append(findParams.Filters, query.Filter{
			Column: query.FieldRoleID,
			Filter: repo.In(roleIDs),
		})
	}

	if v := r.URL.Query().Get("CreatedAt.To"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			logger.Errorf("Error parsing CreatedAt.To: %v", err)
			http.Error(w, "Invalid date format", http.StatusBadRequest)
			return
		}
		findParams.Filters = append(findParams.Filters, query.Filter{
			Column: query.FieldCreatedAt,
			Filter: repo.Lt(t),
		})
	}

	if v := r.URL.Query().Get("CreatedAt.From"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			logger.Errorf("Error parsing CreatedAt.From: %v", err)
			http.Error(w, "Invalid date format", http.StatusBadRequest)
			return
		}
		findParams.Filters = append(findParams.Filters, query.Filter{
			Column: query.FieldCreatedAt,
			Filter: repo.Gt(t),
		})
	}

	// Get users using the query service
	us, total, err := userQueryService.FindUsers(r.Context(), findParams)
	if err != nil {
		logger.Errorf("Error retrieving users: %v", err)
		http.Error(w, "Error retrieving users", http.StatusInternalServerError)
		return
	}

	var groups []*viewmodels.Group
	canListGroups := pageCtx != nil && pageCtx.CanAuthz(groupsAuthzObject, "list")
	if canListGroups {
		groupParams := &query.GroupFindParams{
			Limit:  100,
			Offset: 0,
			SortBy: query.SortBy{
				Fields: []repo.SortByField[query.Field]{
					{Field: query.GroupFieldName, Ascending: true},
				},
			},
			Filters: []query.GroupFilter{},
		}
		var grpErr error
		groups, _, grpErr = groupQueryService.FindGroups(r.Context(), groupParams)
		if grpErr != nil {
			if isAuthzForbidden(grpErr) {
				logger.WithError(grpErr).Warn("group filters unauthorized, hiding sidebar data")
				groups = []*viewmodels.Group{}
			} else {
				logger.Errorf("Error retrieving groups: %v", grpErr)
				http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
				return
			}
		}
	}

	var roleViewModels []*viewmodels.Role
	canListRoles := pageCtx != nil && pageCtx.CanAuthz(rolesAuthzObject, "list")
	if canListRoles {
		roleViewModels, err = roleQueryService.GetRolesWithCounts(r.Context())
		if err != nil {
			if isAuthzForbidden(err) {
				logger.WithError(err).Warn("role filters unauthorized, hiding sidebar data")
				roleViewModels = []*viewmodels.Role{}
			} else {
				logger.Errorf("Error retrieving roles: %v", err)
				http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
				return
			}
		}
	}

	props := &users.IndexPageProps{
		Users:   us,             // Already viewmodels from query service
		Groups:  groups,         // Already viewmodels from query service (when authorized)
		Roles:   roleViewModels, // Mapped from domain roles
		Page:    params.Page,
		PerPage: params.Limit,
		HasMore: total > params.Page*params.Limit,
	}

	if htmx.IsHxRequest(r) {
		if params.Page > 1 {
			templ.Handler(users.UserRows(props), templ.WithStreaming()).ServeHTTP(w, r)
		} else {
			if htmx.Target(r) == "users-table-body" {
				templ.Handler(users.UserRows(props), templ.WithStreaming()).ServeHTTP(w, r)
			} else {
				templ.Handler(users.UsersContent(props), templ.WithStreaming()).ServeHTTP(w, r)
			}
		}
	} else {
		templ.Handler(users.Index(props), templ.WithStreaming()).ServeHTTP(w, r)
	}
}

func (c *UsersController) GetEdit(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
	roleService *services.RoleService,
	groupQueryService *services.GroupQueryService,
	policyService *services.AuthzPolicyService,
) {
	if !ensureUsersAuthz(w, r, "view") {
		return
	}

	ensurePageCapabilities(r, usersAuthzObject, "update", "delete")
	ensurePageCapabilities(r, rolesAuthzObject, "list")
	ensurePageCapabilities(r, groupsAuthzObject, "list")

	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing user ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var roles []role.Role
	if canAuthzCapability(r.Context(), rolesAuthzObject, "list") {
		roles, err = roleService.GetAll(r.Context())
		if err != nil {
			logger.Errorf("Error retrieving roles: %v", err)
			http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
			return
		}
	}
	roleNameToID := make(map[string]uint, len(roles))
	for _, roleEntity := range roles {
		roleNameToID[roleEntity.Name()] = roleEntity.ID()
	}

	var groups []*viewmodels.Group
	if canAuthzCapability(r.Context(), groupsAuthzObject, "list") {
		groupParams := &query.GroupFindParams{
			Limit:   1000,
			Offset:  0,
			Filters: []query.GroupFilter{},
		}
		var grpErr error
		groups, _, grpErr = groupQueryService.FindGroups(r.Context(), groupParams)
		if grpErr != nil {
			logger.Errorf("Error retrieving groups: %v", grpErr)
			http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
			return
		}
	}

	us, err := userService.GetByID(r.Context(), id)
	if err != nil {
		logger.Errorf("Error retrieving user: %v", err)
		http.Error(w, "Error retrieving users", http.StatusInternalServerError)
		return
	}

	canDelete, err := userService.CanUserBeDeleted(r.Context(), id)
	if err != nil {
		logger.Errorf("Error checking if user can be deleted: %v", err)
		http.Error(w, "Error retrieving user information", http.StatusInternalServerError)
		return
	}

	userViewModel := mappers.UserToViewModel(us)
	userViewModel.CanDelete = canDelete

	var policyBoard *users.UserPolicyBoardProps
	if composables.CanUser(r.Context(), permissions.AuthzDebug) == nil {
		board, buildErr := c.buildUserPolicyBoardProps(r.Context(), r, policyService, us, r.URL.Query(), roleNameToID)
		if buildErr != nil {
			logger.WithError(buildErr).Warn("failed to build user policy board")
		} else {
			policyBoard = board
		}
	}

	props := &users.EditFormProps{
		User:                     userViewModel,
		Roles:                    mapping.MapViewModels(roles, mappers.RoleToViewModel),
		Groups:                   groups, // Already viewmodels from query service
		ResourcePermissionGroups: c.resourcePermissionGroups(us.Permissions()...),
		Errors:                   map[string]string{},
		CanDelete:                canDelete,
		PolicyBoard:              policyBoard,
	}
	templ.Handler(users.Edit(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *UsersController) GetPolicies(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
	roleService *services.RoleService,
	policyService *services.AuthzPolicyService,
) {
	if !ensureUsersAuthz(w, r, "view") {
		return
	}
	if !ensureAuthz(w, r, authz.ObjectName("core", "authz"), "debug", nil) {
		return
	}
	id, err := shared.ParseID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ID", "invalid user id")
		return
	}
	us, err := userService.GetByID(r.Context(), id)
	if err != nil {
		logger.WithError(err).Error("failed to load user for policies")
		writeJSONError(w, http.StatusInternalServerError, "USER_NOT_FOUND", "failed to load user")
		return
	}
	roleNameToID := map[string]uint{}
	if canAuthzCapability(r.Context(), rolesAuthzObject, "list") {
		roles, rolesErr := roleService.GetAll(r.Context())
		if rolesErr != nil {
			logger.WithError(rolesErr).Warn("failed to load roles for user policy board")
		} else {
			roleNameToID = make(map[string]uint, len(roles))
			for _, roleEntity := range roles {
				roleNameToID[roleEntity.Name()] = roleEntity.ID()
			}
		}
	}

	board, buildErr := c.buildUserPolicyBoardProps(r.Context(), r, policyService, us, r.URL.Query(), roleNameToID)
	if buildErr != nil {
		logger.WithError(buildErr).Error("failed to build user policy board")
		writeJSONError(w, http.StatusInternalServerError, "POLICY_BOARD_ERROR", "failed to load user policies")
		return
	}
	column := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("column")))
	switch column {
	case "":
		templ.Handler(users.UserPolicyBoard(board), templ.WithStreaming()).ServeHTTP(w, r)
		return
	case "effective":
		templ.Handler(users.UserEffectivePolicies(&board.Effective), templ.WithStreaming()).ServeHTTP(w, r)
		return
	case userPolicyColumnInherited:
		templ.Handler(users.UserPolicyColumn(&board.Inherited), templ.WithStreaming()).ServeHTTP(w, r)
		return
	case userPolicyColumnDirect:
		templ.Handler(users.UserPolicyColumn(&board.Direct), templ.WithStreaming()).ServeHTTP(w, r)
		return
	case userPolicyColumnOverrides:
		templ.Handler(users.UserPolicyColumn(&board.Overrides), templ.WithStreaming()).ServeHTTP(w, r)
		return
	default:
		writeJSONError(w, http.StatusBadRequest, "INVALID_COLUMN", "unknown column")
		return
	}
}

func (c *UsersController) GetNew(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	roleService *services.RoleService,
	groupQueryService *services.GroupQueryService,
) {
	if !ensureUsersAuthz(w, r, "create") {
		return
	}

	ensurePageCapabilities(r, usersAuthzObject, "create")
	ensurePageCapabilities(r, rolesAuthzObject, "list")
	ensurePageCapabilities(r, groupsAuthzObject, "list")

	var roles []role.Role
	var err error
	if canAuthzCapability(r.Context(), rolesAuthzObject, "list") {
		roles, err = roleService.GetAll(r.Context())
		if err != nil {
			logger.Errorf("Error retrieving roles: %v", err)
			http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
			return
		}
	}

	var groups []*viewmodels.Group
	if canAuthzCapability(r.Context(), groupsAuthzObject, "list") {
		groupParams := &query.GroupFindParams{
			Limit:   1000,
			Offset:  0,
			Filters: []query.GroupFilter{},
		}
		var grpErr error
		groups, _, grpErr = groupQueryService.FindGroups(r.Context(), groupParams)
		if grpErr != nil {
			logger.Errorf("Error retrieving groups: %v", grpErr)
			http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
			return
		}
	}

	props := &users.CreateFormProps{
		User:                     viewmodels.User{},
		Roles:                    mapping.MapViewModels(roles, mappers.RoleToViewModel),
		Groups:                   groups, // Already viewmodels from query service
		ResourcePermissionGroups: c.resourcePermissionGroups(),
		Errors:                   map[string]string{},
	}
	templ.Handler(users.New(props), templ.WithStreaming()).ServeHTTP(w, r)
}

func (c *UsersController) Create(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
	roleService *services.RoleService,
	groupQueryService *services.GroupQueryService,
) {
	if !ensureUsersAuthz(w, r, "create") {
		return
	}

	ensurePageCapabilities(r, usersAuthzObject, "create")
	ensurePageCapabilities(r, rolesAuthzObject, "list")
	ensurePageCapabilities(r, groupsAuthzObject, "list")

	respondWithForm := func(errors map[string]string, dto *dtos.CreateUserDTO) {
		ctx := r.Context()

		var roles []role.Role
		var err error
		if canAuthzCapability(ctx, rolesAuthzObject, "list") {
			roles, err = roleService.GetAll(ctx)
			if err != nil {
				logger.Errorf("Error retrieving roles: %v", err)
				http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
				return
			}
		}

		var groups []*viewmodels.Group
		if canAuthzCapability(ctx, groupsAuthzObject, "list") {
			groupParams := &query.GroupFindParams{
				Limit:   1000,
				Offset:  0,
				Filters: []query.GroupFilter{},
			}
			var grpErr error
			groups, _, grpErr = groupQueryService.FindGroups(ctx, groupParams)
			if grpErr != nil {
				logger.Errorf("Error retrieving groups: %v", grpErr)
				http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
				return
			}
		}

		var selectedRoles []*viewmodels.Role
		for _, role := range roles {
			if slices.Contains(dto.RoleIDs, role.ID()) {
				selectedRoles = append(selectedRoles, mappers.RoleToViewModel(role))
			}
		}

		props := &users.CreateFormProps{
			User: viewmodels.User{
				FirstName:  dto.FirstName,
				LastName:   dto.LastName,
				MiddleName: dto.MiddleName,
				Email:      dto.Email,
				Phone:      dto.Phone,
				GroupIDs:   dto.GroupIDs,
				Roles:      selectedRoles,
				Language:   dto.Language,
				AvatarID:   fmt.Sprint(dto.AvatarID),
			},
			Roles:                    mapping.MapViewModels(roles, mappers.RoleToViewModel),
			Groups:                   groups, // Already viewmodels from query service
			ResourcePermissionGroups: c.resourcePermissionGroups(),
			Errors:                   errors,
		}

		templ.Handler(users.CreateForm(props), templ.WithStreaming()).ServeHTTP(w, r)
	}

	dto, err := composables.UseForm(&dtos.CreateUserDTO{}, r)
	if err != nil {
		logger.Errorf("Error parsing form: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if errs, ok := dto.Ok(r.Context()); !ok {
		respondWithForm(errs, dto)
		return
	}

	tenantID, err := composables.UseTenantID(r.Context())
	if err != nil {
		logger.Errorf("Error getting tenant: %v", err)
		http.Error(w, "Error getting tenant", http.StatusInternalServerError)
		return
	}

	userEntity, err := dto.ToEntity(tenantID)
	if err != nil {
		logger.Errorf("Error converting DTO to entity: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := userService.Create(r.Context(), userEntity); err != nil {
		var errs *validators.ValidationError
		if errors.As(err, &errs) {
			respondWithForm(errs.Fields, dto)
			return
		}

		logger.Errorf("Error creating user: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shared.Redirect(w, r, c.basePath)
}

func (c *UsersController) Update(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
	roleService *services.RoleService,
	groupQueryService *services.GroupQueryService,
	permissionService *services.PermissionService,
) {
	if !ensureUsersAuthz(w, r, "update") {
		return
	}

	ensurePageCapabilities(r, usersAuthzObject, "update", "delete")
	ensurePageCapabilities(r, rolesAuthzObject, "list")
	ensurePageCapabilities(r, groupsAuthzObject, "list")

	ctx := r.Context()

	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing user ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Errorf("Error parsing form: %v", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	dto, err := composables.UseForm(&dtos.UpdateUserDTO{}, r)
	if err != nil {
		logger.Errorf("Error parsing form: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	respondWithForm := func(errors map[string]string, dto *dtos.UpdateUserDTO) {
		us, err := userService.GetByID(ctx, id)
		if err != nil {
			logger.Errorf("Error retrieving user: %v", err)
			http.Error(w, "Error retrieving user", http.StatusInternalServerError)
			return
		}

		var roles []role.Role
		if canAuthzCapability(ctx, rolesAuthzObject, "list") {
			roles, err = roleService.GetAll(ctx)
			if err != nil {
				logger.Errorf("Error retrieving roles: %v", err)
				http.Error(w, "Error retrieving roles", http.StatusInternalServerError)
				return
			}
		}

		var selectedRoles []*viewmodels.Role
		for _, role := range roles {
			if slices.Contains(dto.RoleIDs, role.ID()) {
				selectedRoles = append(selectedRoles, mappers.RoleToViewModel(role))
			}
		}

		var groups []*viewmodels.Group
		if canAuthzCapability(ctx, groupsAuthzObject, "list") {
			groupParams := &query.GroupFindParams{
				Limit:   1000,
				Offset:  0,
				Filters: []query.GroupFilter{},
			}
			var grpErr error
			groups, _, grpErr = groupQueryService.FindGroups(ctx, groupParams)
			if grpErr != nil {
				logger.Errorf("Error retrieving groups: %v", grpErr)
				http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
				return
			}
		}

		canDelete, err := userService.CanUserBeDeleted(ctx, id)
		if err != nil {
			logger.Errorf("Error checking if user can be deleted: %v", err)
			http.Error(w, "Error retrieving user information", http.StatusInternalServerError)
			return
		}

		var avatar *viewmodels.Upload
		if us.Avatar() != nil {
			avatar = mappers.UploadToViewModel(us.Avatar())
		}

		props := &users.EditFormProps{
			User: &viewmodels.User{
				ID:          strconv.FormatUint(uint64(id), 10),
				FirstName:   dto.FirstName,
				LastName:    dto.LastName,
				MiddleName:  dto.MiddleName,
				Email:       dto.Email,
				Phone:       dto.Phone,
				Avatar:      avatar,
				Language:    dto.Language,
				LastAction:  us.LastAction().Format(time.RFC3339),
				CreatedAt:   us.CreatedAt().Format(time.RFC3339),
				Roles:       selectedRoles,
				GroupIDs:    dto.GroupIDs,
				Permissions: mapping.MapViewModels(us.Permissions(), mappers.PermissionToViewModel),
				AvatarID:    strconv.FormatUint(uint64(dto.AvatarID), 10),
				CanDelete:   canDelete,
			},
			Roles:                    mapping.MapViewModels(roles, mappers.RoleToViewModel),
			Groups:                   groups, // Already viewmodels from query service
			ResourcePermissionGroups: c.resourcePermissionGroups(us.Permissions()...),
			Errors:                   errors,
			CanDelete:                canDelete,
		}
		templ.Handler(users.EditForm(props), templ.WithStreaming()).ServeHTTP(w, r)
	}

	if errs, ok := dto.Ok(ctx); !ok {
		respondWithForm(errs, dto)
		return
	}

	userEntity, err := userService.GetByID(ctx, id)
	if err != nil {
		logger.Errorf("Error converting DTO to entity: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	roles := make([]role.Role, 0, len(dto.RoleIDs))
	for _, rID := range dto.RoleIDs {
		r, err := roleService.GetByID(ctx, rID)
		if err != nil {
			logger.Errorf("Error getting role: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		roles = append(roles, r)
	}

	permissionIDs := r.Form["PermissionIDs"]
	permissions := make([]*permission.Permission, 0, len(permissionIDs))
	for _, permID := range permissionIDs {
		if permID == "" {
			continue
		}
		perm, err := permissionService.GetByID(ctx, permID)
		if err != nil {
			logger.Warnf("Error retrieving permission: %v", err)
			continue
		}
		permissions = append(permissions, perm)
	}

	userEntity, err = dto.Apply(userEntity, roles, permissions)
	if err != nil {
		logger.Errorf("Error updating user: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := userService.Update(ctx, userEntity); err != nil {
		var errs *validators.ValidationError
		if errors.As(err, &errs) {
			respondWithForm(errs.Fields, dto)
			return
		}

		logger.Errorf("Error creating user: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shared.Redirect(w, r, c.basePath)
}

func (c *UsersController) Delete(
	r *http.Request,
	w http.ResponseWriter,
	logger *logrus.Entry,
	userService *services.UserService,
) {
	if !ensureUsersAuthz(w, r, "delete") {
		return
	}

	id, err := shared.ParseID(r)
	if err != nil {
		logger.Errorf("Error parsing user ID: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := userService.Delete(r.Context(), id); err != nil {
		logger.Errorf("Error deleting user: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shared.Redirect(w, r, c.basePath)
}

func canAuthzCapability(ctx context.Context, object, action string) bool {
	pageCtx, ok := composables.TryUsePageCtx(ctx)
	if !ok || pageCtx == nil {
		return false
	}
	return pageCtx.CanAuthz(object, action)
}

func (c *UsersController) buildUserPolicyBoardProps(
	ctx context.Context,
	r *http.Request,
	policyService *services.AuthzPolicyService,
	us user.User,
	values url.Values,
	roleNameToID map[string]uint,
) (*users.UserPolicyBoardProps, error) {
	subject := authzSubjectForUser(us.TenantID(), us)
	defaultDomain := authz.DomainFromTenant(us.TenantID())
	activeDomain := strings.TrimSpace(values.Get("domain"))
	if activeDomain == "" {
		activeDomain = defaultDomain
	}

	entries, err := policyService.Policies(ctx)
	if err != nil {
		return nil, err
	}
	selectorOpts := buildAuthzSelectorOptions(entries)
	subjectEntries := filterPolicies(entries, PolicyListParams{Subject: subject})

	tenantID := us.TenantID()
	var stagedEntries []dtos.StagedPolicyEntry
	if currentUser, err := composables.UseUser(ctx); err == nil && currentUser != nil {
		stagedEntries = c.stageStore.List(policyStageKey(currentUser.ID(), tenantID), subject, activeDomain)
	}

	baseURL := fmt.Sprintf("%s/%d/policies", c.basePath, us.ID())
	canStage := composables.CanUser(ctx, permissions.AuthzRequestsWrite) == nil &&
		composables.CanUser(ctx, permissions.UserUpdate) == nil
	canDebug := composables.CanUser(ctx, permissions.AuthzDebug) == nil

	inheritedParams := parseUserPolicyParams(values, 20)
	directParams := parseUserPolicyParams(values, 20)
	overrideParams := parseUserPolicyParams(values, 20)

	inheritedParams.Subject = subject
	directParams.Subject = subject
	overrideParams.Subject = subject

	inheritedParams.Type = "g"
	if inheritedParams.Domain == "" {
		inheritedParams.Domain = activeDomain
	}

	directParams.Type = "p"
	if directParams.Domain == "" {
		directParams.Domain = activeDomain
	}

	overrideParams.Type = "p"

	inheritedColumn, err := c.buildUserPolicyColumnProps(
		subjectEntries,
		stagedEntries,
		inheritedParams,
		userPolicyColumnInherited,
		defaultDomain,
		baseURL,
		selectorOpts,
		canStage,
		canDebug,
		roleNameToID,
	)
	if err != nil {
		return nil, err
	}
	directColumn, err := c.buildUserPolicyColumnProps(
		subjectEntries,
		stagedEntries,
		directParams,
		userPolicyColumnDirect,
		defaultDomain,
		baseURL,
		selectorOpts,
		canStage,
		canDebug,
		roleNameToID,
	)
	if err != nil {
		return nil, err
	}
	overrideColumn, err := c.buildUserPolicyColumnProps(
		subjectEntries,
		stagedEntries,
		overrideParams,
		userPolicyColumnOverrides,
		defaultDomain,
		baseURL,
		selectorOpts,
		canStage,
		canDebug,
		roleNameToID,
	)
	if err != nil {
		return nil, err
	}

	effectiveParams := parseUserPolicyParams(values, 20)
	effectiveProps := buildUserEffectivePoliciesProps(entries, subject, defaultDomain, effectiveParams, roleNameToID, baseURL)

	return &users.UserPolicyBoardProps{
		Subject:       subject,
		Domain:        activeDomain,
		DefaultDomain: defaultDomain,
		StageTotal:    len(stagedEntries),
		StageSummary:  summarizeStagedEntries(stagedEntries),
		StagePreview:  buildWorkspacePreview(stagedEntries),
		Effective:     effectiveProps,
		Inherited:     inheritedColumn,
		Direct:        directColumn,
		Overrides:     overrideColumn,
		BaseURL:       baseURL,
		CanStage:      canStage,
		CanDebug:      canDebug,
	}, nil
}

func (c *UsersController) buildUserPolicyColumnProps(
	baseEntries []services.PolicyEntry,
	stagedEntries []dtos.StagedPolicyEntry,
	params PolicyListParams,
	column string,
	defaultDomain string,
	baseURL string,
	selectorOpts authzSelectorOptions,
	canStage bool,
	canDebug bool,
	roleNameToID map[string]uint,
) (users.UserPolicyColumnProps, error) {
	filteredBase := filterUserPolicyEntriesByColumn(baseEntries, column, defaultDomain, params)
	filteredStage := filterUserStagedEntriesByColumn(stagedEntries, column, defaultDomain, params)

	matrixEntries, total := mergePolicyMatrixEntries(filteredBase, filteredStage, params)
	entries := make([]users.UserPolicyEntry, 0, len(matrixEntries))
	for _, entry := range matrixEntries {
		rolePoliciesURL := ""
		if entry.Type == "g" && roleNameToID != nil {
			roleName := strings.TrimPrefix(strings.TrimSpace(entry.Object), "role:")
			if roleName != "" {
				if roleID, ok := roleNameToID[roleName]; ok && roleID > 0 {
					rolePoliciesURL = fmt.Sprintf("/roles/%d/policies?domain=%s", roleID, url.QueryEscape(entry.Domain))
				}
			}
		}
		entries = append(entries, users.UserPolicyEntry{
			PolicyEntryResponse: entry.PolicyEntryResponse,
			StageID:             entry.StageID,
			StageKind:           entry.StageKind,
			Staged:              entry.Staged,
			StageOnly:           entry.StageOnly,
			RolePoliciesURL:     rolePoliciesURL,
		})
	}

	return users.UserPolicyColumnProps{
		Column:        column,
		Entries:       entries,
		Total:         total,
		Page:          params.Page,
		Limit:         params.Limit,
		StageTotal:    len(filteredStage),
		Subject:       params.Subject,
		Domain:        params.Domain,
		DefaultDomain: defaultDomain,
		Type:          params.Type,
		Search:        params.Search,
		DomainFilter:  params.Domain,
		ObjectOptions: selectorOpts.Objects,
		ActionOptions: selectorOpts.Actions,
		RoleOptions:   selectorOpts.Roles,
		CanDebug:      canDebug,
		CanStage:      canStage,
		BaseURL:       baseURL,
	}, nil
}

func parseUserPolicyParams(values url.Values, defaultLimit int) PolicyListParams {
	params := PolicyListParams{
		Page:      1,
		Limit:     defaultLimit,
		SortField: "object",
		SortAsc:   true,
	}
	if page := strings.TrimSpace(values.Get("page")); page != "" {
		val, err := strconv.Atoi(page)
		if err == nil && val > 0 {
			params.Page = val
		}
	}
	if limit := strings.TrimSpace(values.Get("limit")); limit != "" {
		val, err := strconv.Atoi(limit)
		if err == nil && val > 0 {
			params.Limit = val
		}
	}
	params.Domain = strings.TrimSpace(values.Get("domain"))
	params.Search = strings.TrimSpace(values.Get("q"))
	return params
}

func filterUserPolicyEntriesByColumn(
	entries []services.PolicyEntry,
	column string,
	defaultDomain string,
	params PolicyListParams,
) []services.PolicyEntry {
	filtered := make([]services.PolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if params.Subject != "" && entry.Subject != params.Subject {
			continue
		}
		switch column {
		case userPolicyColumnInherited:
			domain := params.Domain
			if domain == "" {
				domain = defaultDomain
			}
			if entry.Type != "g" || entry.Domain != domain {
				continue
			}
		case userPolicyColumnDirect:
			domain := params.Domain
			if domain == "" {
				domain = defaultDomain
			}
			if entry.Type != "p" || entry.Domain != domain {
				continue
			}
		case userPolicyColumnOverrides:
			if entry.Type != "p" {
				continue
			}
			if params.Domain != "" {
				if entry.Domain != params.Domain {
					continue
				}
			} else if entry.Domain == defaultDomain {
				continue
			}
		default:
			continue
		}
		if params.Search != "" {
			search := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(entry.Object), search) && !strings.Contains(strings.ToLower(entry.Action), search) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func filterUserStagedEntriesByColumn(
	entries []dtos.StagedPolicyEntry,
	column string,
	defaultDomain string,
	params PolicyListParams,
) []dtos.StagedPolicyEntry {
	filtered := make([]dtos.StagedPolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if params.Subject != "" && entry.Subject != params.Subject {
			continue
		}
		switch column {
		case userPolicyColumnInherited:
			domain := params.Domain
			if domain == "" {
				domain = defaultDomain
			}
			if entry.Type != "g" || entry.Domain != domain {
				continue
			}
		case userPolicyColumnDirect:
			domain := params.Domain
			if domain == "" {
				domain = defaultDomain
			}
			if entry.Type != "p" || entry.Domain != domain {
				continue
			}
		case userPolicyColumnOverrides:
			if entry.Type != "p" {
				continue
			}
			if params.Domain != "" {
				if entry.Domain != params.Domain {
					continue
				}
			} else if entry.Domain == defaultDomain {
				continue
			}
		default:
			continue
		}
		if params.Search != "" {
			search := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(entry.Object), search) && !strings.Contains(strings.ToLower(entry.Action), search) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
