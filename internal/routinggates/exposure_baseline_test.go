package routinggates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	internalassets "github.com/iota-uz/iota-sdk/internal/assets"
	internalserver "github.com/iota-uz/iota-sdk/internal/server"
	"github.com/iota-uz/iota-sdk/modules"
	corepersistence "github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	corecontrollers "github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
	"github.com/iota-uz/iota-sdk/pkg/routing"
	pkgserver "github.com/iota-uz/iota-sdk/pkg/server"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestExposureBaseline_Production_DoesNotRegisterDevPlaygroundOrTestRoutes(t *testing.T) {
	srv := buildMainServerHTTPServer(t)
	router := srv.Router()

	paths := collectRoutePaths(t, router)

	var offending []string
	for _, p := range paths {
		switch {
		case routing.HasPathPrefixOnBoundary(p, "/_dev"),
			routing.HasPathPrefixOnBoundary(p, "/playground"),
			routing.HasPathPrefixOnBoundary(p, "/__test__"):
			offending = append(offending, p)
		}
	}

	if len(offending) > 0 {
		sort.Strings(offending)
		t.Fatalf("生产默认不应注册 dev/playground/test 路由（/ _dev, /playground, /__test__）：\n%s", strings.Join(offending, "\n"))
	}
}

func TestExposureBaseline_UI404_NotForcedJSON(t *testing.T) {
	srv := buildMainServerHTTPServer(t)
	router := srv.Router()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/__nonexistent_ui__", nil)
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.NotEqual(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestExposureBaseline_OpsGuard_Production_DeniesWithoutAuth(t *testing.T) {
	t.Setenv("ROUTING_ALLOWLIST_PATH", routing.DefaultAllowlistPath())

	conf := &configuration.Configuration{
		GoAppEnvironment: configuration.Production,
		OpsGuardEnabled:  true,
		RealIPHeader:     "X-Real-IP",
	}

	r := mux.NewRouter()
	r.Use(middleware.OpsGuard(conf, "server"))
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestExposureBaseline_OpsGuard_Production_AllowsWithToken(t *testing.T) {
	t.Setenv("ROUTING_ALLOWLIST_PATH", routing.DefaultAllowlistPath())

	conf := &configuration.Configuration{
		GoAppEnvironment: configuration.Production,
		OpsGuardEnabled:  true,
		RealIPHeader:     "X-Real-IP",
		OpsGuardToken:    "secret",
	}

	r := mux.NewRouter()
	r.Use(middleware.OpsGuard(conf, "server"))
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	req.Header.Set("X-Ops-Token", "secret")
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
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
	return strings.TrimPrefix(regexp, "^")
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

func newLazyPool(t *testing.T, opts string) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
	})
	return pool
}
