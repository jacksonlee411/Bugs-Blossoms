package routelint

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	internalassets "github.com/iota-uz/iota-sdk/internal/assets"
	internalserver "github.com/iota-uz/iota-sdk/internal/server"
	"github.com/iota-uz/iota-sdk/modules"
	"github.com/iota-uz/iota-sdk/modules/core"
	corepersistence "github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/query"
	coreassets "github.com/iota-uz/iota-sdk/modules/core/presentation/assets"
	corecontrollers "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/modules/core/validators"
	"github.com/iota-uz/iota-sdk/modules/superadmin"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/routing"
	pkgserver "github.com/iota-uz/iota-sdk/pkg/server"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestServerRoutes_NoUnversionedAPIExceptAllowlist(t *testing.T) {
	srv := buildMainServerHTTPServer(t)
	router := srv.Router()

	rules, err := routing.LoadAllowlist("", "server")
	require.NoError(t, err)

	assertNoUnversionedAPIs(t, router, routing.NewClassifier(rules))
}

func TestSuperadminRoutes_NoUnversionedAPIExceptAllowlist(t *testing.T) {
	srv := buildSuperadminHTTPServer(t)
	router := srv.Router()

	rules, err := routing.LoadAllowlist("", "superadmin")
	require.NoError(t, err)

	assertNoUnversionedAPIs(t, router, routing.NewClassifier(rules))
}

func TestServerRoutes_TopLevelExceptionsMustBeAllowlisted(t *testing.T) {
	srv := buildMainServerHTTPServer(t)
	router := srv.Router()

	rules, err := routing.LoadAllowlist("", "server")
	require.NoError(t, err)

	assertTopLevelExceptionsAreAllowlisted(t, router, routing.NewClassifier(rules))
}

func TestSuperadminRoutes_TopLevelExceptionsMustBeAllowlisted(t *testing.T) {
	srv := buildSuperadminHTTPServer(t)
	router := srv.Router()

	rules, err := routing.LoadAllowlist("", "superadmin")
	require.NoError(t, err)

	assertTopLevelExceptionsAreAllowlisted(t, router, routing.NewClassifier(rules))
}

func assertNoUnversionedAPIs(t *testing.T, router *mux.Router, classifier *routing.Classifier) {
	t.Helper()

	paths := collectRoutePaths(t, router)

	offending := make([]string, 0, len(paths))
	for _, p := range paths {
		if !routing.HasPathPrefixOnBoundary(p, "/api") {
			continue
		}
		if routing.HasPathPrefixOnBoundary(p, "/api/v1") {
			continue
		}
		if _, ok := classifier.MatchAllowlist(p); ok {
			continue
		}
		offending = append(offending, p)
	}

	if len(offending) > 0 {
		sort.Strings(offending)
		t.Fatalf("发现非版本化 /api 前缀路由（未在 allowlist 登记）：\n%s", strings.Join(offending, "\n"))
	}
}

func assertTopLevelExceptionsAreAllowlisted(t *testing.T, router *mux.Router, classifier *routing.Classifier) {
	t.Helper()

	paths := collectRoutePaths(t, router)
	moduleNames := loadModuleNames(t)

	offendingSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" || p == "/" {
			continue
		}
		segment := firstPathSegment(p)
		if segment == "" {
			continue
		}
		if _, ok := moduleNames[segment]; ok {
			continue
		}
		if _, ok := classifier.MatchAllowlist(p); ok {
			continue
		}
		offendingSet[p] = struct{}{}
	}

	if len(offendingSet) > 0 {
		offending := make([]string, 0, len(offendingSet))
		for p := range offendingSet {
			offending = append(offending, p)
		}
		sort.Strings(offending)
		t.Fatalf("发现未登记 allowlist 的顶层例外/legacy 前缀路由：\n%s", strings.Join(offending, "\n"))
	}
}

func collectRoutePaths(t *testing.T, router *mux.Router) []string {
	t.Helper()

	var paths []string
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		p := routePath(route)
		if strings.TrimSpace(p) != "" {
			paths = append(paths, p)
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(paths)
	return paths
}

func routePath(route *mux.Route) string {
	if route == nil {
		return ""
	}
	if tmpl, err := route.GetPathTemplate(); err == nil {
		return tmpl
	}
	regexp, err := route.GetPathRegexp()
	if err != nil {
		return ""
	}
	result := strings.TrimPrefix(regexp, "^")
	return strings.TrimSuffix(result, "$")
}

func firstPathSegment(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	segment, _, _ := strings.Cut(path, "/")
	return segment
}

func loadModuleNames(t *testing.T) map[string]struct{} {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	repoRoot, ok := findGoModRoot(wd)
	require.True(t, ok, "failed to locate go.mod root from %q", wd)

	entries, err := os.ReadDir(filepath.Join(repoRoot, "modules"))
	require.NoError(t, err)

	result := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := strings.TrimSpace(e.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func findGoModRoot(start string) (string, bool) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func buildMainServerHTTPServer(t *testing.T) *pkgserver.HTTPServer {
	t.Helper()

	conf := configuration.Use()
	logger := conf.Logger()

	pool := newLazyPool(t, conf.Database.Opts)

	bundle := application.LoadBundle()
	app := application.New(&application.ApplicationOptions{
		Pool:     pool,
		Bundle:   bundle,
		EventBus: eventbus.NewEventPublisher(logger),
		Logger:   logger,
		Huber: application.NewHub(&application.HuberOptions{
			Pool:           pool,
			Logger:         logger,
			Bundle:         bundle,
			UserRepository: corepersistence.NewUserRepository(corepersistence.NewUploadRepository()),
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		}),
	})

	require.NoError(t, modules.Load(app, modules.BuiltInModules...))

	app.RegisterNavItems(modules.NavLinks...)
	app.RegisterHashFsAssets(internalassets.HashFS)
	app.RegisterControllers(
		corecontrollers.NewStaticFilesController(app.HashFsAssets()),
		corecontrollers.NewGraphQLController(app),
	)

	srv, err := internalserver.Default(&internalserver.DefaultOptions{
		Logger:        logger,
		Configuration: conf,
		Application:   app,
		Pool:          pool,
		Entrypoint:    "server",
	})
	require.NoError(t, err)

	return srv
}

func buildSuperadminHTTPServer(t *testing.T) *pkgserver.HTTPServer {
	t.Helper()

	conf := configuration.Use()
	logger := conf.Logger()

	pool := newLazyPool(t, conf.Database.Opts)

	bundle := application.LoadBundle()
	app := application.New(&application.ApplicationOptions{
		Pool:     pool,
		Bundle:   bundle,
		EventBus: eventbus.NewEventPublisher(logger),
		Logger:   logger,
		Huber: application.NewHub(&application.HuberOptions{
			Pool:           pool,
			Logger:         logger,
			Bundle:         bundle,
			UserRepository: corepersistence.NewUserRepository(corepersistence.NewUploadRepository()),
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		}),
	})

	app.Migrations().RegisterSchema(&core.MigrationFiles)
	app.RegisterLocaleFiles(&core.LocaleFiles)

	fsStorage, err := corepersistence.NewFSStorage()
	require.NoError(t, err)

	uploadRepo := corepersistence.NewUploadRepository()
	userRepo := corepersistence.NewUserRepository(uploadRepo)
	roleRepo := corepersistence.NewRoleRepository()
	tenantRepo := corepersistence.NewTenantRepository()
	permRepo := corepersistence.NewPermissionRepository()
	userQueryRepo := query.NewPgUserQueryRepository()
	groupQueryRepo := query.NewPgGroupQueryRepository()
	userValidator := validators.NewUserValidator(userRepo)

	tenantService := services.NewTenantService(tenantRepo)
	uploadService := services.NewUploadService(uploadRepo, fsStorage, app.EventPublisher())

	app.RegisterServices(
		uploadService,
		services.NewUserService(userRepo, userValidator, app.EventPublisher()),
		services.NewUserQueryService(userQueryRepo),
		services.NewGroupQueryService(groupQueryRepo),
		services.NewSessionService(corepersistence.NewSessionRepository(), app.EventPublisher()),
		services.NewExcelExportService(app.DB(), uploadService),
	)
	app.RegisterServices(
		services.NewAuthService(app),
		services.NewCurrencyService(corepersistence.NewCurrencyRepository(), app.EventPublisher()),
		services.NewRoleService(roleRepo, app.EventPublisher()),
		tenantService,
		services.NewPermissionService(permRepo, app.EventPublisher()),
		services.NewGroupService(corepersistence.NewGroupRepository(userRepo, roleRepo), app.EventPublisher()),
	)

	app.RegisterControllers(
		corecontrollers.NewLoginController(app),
		corecontrollers.NewLogoutController(app),
		corecontrollers.NewAccountController(app),
		corecontrollers.NewUploadController(app),
	)

	app.RegisterHashFsAssets(coreassets.HashFS)

	superadminModule := superadmin.NewModule(&superadmin.ModuleOptions{})
	require.NoError(t, superadminModule.Register(app))

	app.RegisterNavItems(superadmin.NavItems...)

	app.RegisterHashFsAssets(internalassets.HashFS)
	app.RegisterControllers(
		corecontrollers.NewStaticFilesController(app.HashFsAssets()),
	)

	srv, err := internalserver.Default(&internalserver.DefaultOptions{
		Logger:        logger,
		Configuration: conf,
		Application:   app,
		Pool:          pool,
		Entrypoint:    "superadmin",
	})
	require.NoError(t, err)

	return srv
}

func newLazyPool(t *testing.T, opts string) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}
