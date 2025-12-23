package configuration

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/logging"

	"github.com/caarlos0/env/v11"
	"github.com/iota-uz/utils/fs"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

const Production = "production"

var singleton = sync.OnceValue(func() *Configuration {
	c := &Configuration{}
	if err := c.load([]string{".env", ".env.local"}); err != nil {
		c.Unload()
		panic(err)
	}
	return c
})

func LoadEnv(envFiles []string) (int, error) {
	exists := make([]bool, len(envFiles))
	for i, file := range envFiles {
		if fs.FileExists(file) {
			exists[i] = true
		}
	}

	existingFiles := make([]string, 0, len(envFiles))
	for i, file := range envFiles {
		if exists[i] {
			existingFiles = append(existingFiles, file)
		}
	}

	if len(existingFiles) == 0 {
		return 0, nil
	}

	return len(existingFiles), godotenv.Load(existingFiles...)
}

type DatabaseOptions struct {
	Opts     string `env:"-"`
	Name     string `env:"DB_NAME" envDefault:"iota_erp"`
	Host     string `env:"DB_HOST" envDefault:"localhost"`
	Port     string `env:"DB_PORT" envDefault:"5432"`
	User     string `env:"DB_USER" envDefault:"postgres"`
	Password string `env:"DB_PASSWORD" envDefault:"postgres"`
}

func (d *DatabaseOptions) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s password=%s sslmode=disable",
		d.Host, d.Port, d.User, d.Name, d.Password,
	)
}

type GoogleOptions struct {
	RedirectURL  string `env:"GOOGLE_REDIRECT_URL"`
	ClientID     string `env:"GOOGLE_CLIENT_ID"`
	ClientSecret string `env:"GOOGLE_CLIENT_SECRET"`
}

type LokiOptions struct {
	URL     string `env:"LOKI_URL"`
	AppName string `env:"LOKI_APP_NAME" envDefault:"sdk"`
	LogPath string `env:"LOG_PATH" envDefault:"./logs/app.log"`
}

type OpenTelemetryOptions struct {
	Enabled     bool   `env:"OTEL_ENABLED" envDefault:"false"`
	TempoURL    string `env:"OTEL_TEMPO_URL" envDefault:"localhost:4318"`
	ServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"sdk"`
}

type PrometheusOptions struct {
	Enabled bool   `env:"PROMETHEUS_METRICS_ENABLED" envDefault:"false"`
	Path    string `env:"PROMETHEUS_METRICS_PATH" envDefault:"/debug/prometheus"`
}

type RateLimitOptions struct {
	Enabled   bool   `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	GlobalRPS int    `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"1000"`
	Storage   string `env:"RATE_LIMIT_STORAGE" envDefault:"memory"` // memory or redis
	RedisURL  string `env:"RATE_LIMIT_REDIS_URL"`
}

// Validate checks the rate limit configuration for errors
func (r *RateLimitOptions) Validate() error {
	if r.GlobalRPS < 0 {
		return fmt.Errorf("rate limit GlobalRPS must be non-negative, got %d", r.GlobalRPS)
	}
	if r.GlobalRPS > 1000000 {
		return fmt.Errorf("rate limit GlobalRPS too high, maximum is 1,000,000, got %d", r.GlobalRPS)
	}
	if r.Storage != "memory" && r.Storage != "redis" {
		return fmt.Errorf("rate limit Storage must be 'memory' or 'redis', got '%s'", r.Storage)
	}
	if r.Storage == "redis" && r.RedisURL == "" {
		return fmt.Errorf("rate limit RedisURL is required when Storage is 'redis'")
	}
	return nil
}

type AuthzOptions struct {
	ModelPath      string `env:"AUTHZ_MODEL_PATH" envDefault:"config/access/model.conf"`
	PolicyPath     string `env:"AUTHZ_POLICY_PATH" envDefault:"config/access/policy.csv"`
	PolicyDir      string `env:"AUTHZ_POLICY_DIR" envDefault:"config/access/policies"`
	FlagConfigPath string `env:"AUTHZ_FLAG_CONFIG" envDefault:"config/access/authz_flags.yaml"`
	FixturesPath   string `env:"AUTHZ_FIXTURES_PATH" envDefault:"config/access/fixtures"`
	Mode           string `env:"AUTHZ_MODE" envDefault:"shadow"`
}

type OutboxOptions struct {
	RelayEnabled         bool          `env:"OUTBOX_RELAY_ENABLED" envDefault:"true"`
	RelayTables          string        `env:"OUTBOX_RELAY_TABLES" envDefault:""`
	RelayPollInterval    time.Duration `env:"OUTBOX_RELAY_POLL_INTERVAL" envDefault:"1s"`
	RelayBatchSize       int           `env:"OUTBOX_RELAY_BATCH_SIZE" envDefault:"100"`
	RelayLockTTL         time.Duration `env:"OUTBOX_RELAY_LOCK_TTL" envDefault:"60s"`
	RelayMaxAttempts     int           `env:"OUTBOX_RELAY_MAX_ATTEMPTS" envDefault:"25"`
	RelaySingleActive    bool          `env:"OUTBOX_RELAY_SINGLE_ACTIVE" envDefault:"true"`
	RelayDispatchTimeout time.Duration `env:"OUTBOX_RELAY_DISPATCH_TIMEOUT" envDefault:"30s"`

	LastErrorMaxBytes int `env:"OUTBOX_LAST_ERROR_MAX_BYTES" envDefault:"2048"`

	CleanerEnabled       bool          `env:"OUTBOX_CLEANER_ENABLED" envDefault:"true"`
	CleanerTables        string        `env:"OUTBOX_CLEANER_TABLES" envDefault:""`
	CleanerInterval      time.Duration `env:"OUTBOX_CLEANER_INTERVAL" envDefault:"1m"`
	CleanerRetention     time.Duration `env:"OUTBOX_CLEANER_RETENTION" envDefault:"168h"`
	CleanerDeadRetention time.Duration `env:"OUTBOX_CLEANER_DEAD_RETENTION" envDefault:"0"`
}

type Configuration struct {
	Database         DatabaseOptions
	Google           GoogleOptions
	Loki             LokiOptions
	OpenTelemetry    OpenTelemetryOptions
	Prometheus       PrometheusOptions
	RateLimit        RateLimitOptions
	Authz            AuthzOptions
	Outbox           OutboxOptions
	ActionLogEnabled bool `env:"ACTION_LOG_ENABLED" envDefault:"false"`

	RedisURL         string        `env:"REDIS_URL" envDefault:"localhost:6379"`
	MigrationsDir    string        `env:"MIGRATIONS_DIR" envDefault:"migrations"`
	ServerPort       int           `env:"PORT" envDefault:"3200"`
	SessionDuration  time.Duration `env:"SESSION_DURATION" envDefault:"720h"`
	GoAppEnvironment string        `env:"GO_APP_ENV" envDefault:"development"`
	SocketAddress    string        `env:"-"`
	OpenAIKey        string        `env:"OPENAI_KEY"`
	UploadsPath      string        `env:"UPLOADS_PATH" envDefault:"static"`
	Domain           string        `env:"DOMAIN" envDefault:"localhost"`
	Origin           string        `env:"ORIGIN" envDefault:"http://localhost:3200"`
	PageSize         int           `env:"PAGE_SIZE" envDefault:"25"`
	MaxPageSize      int           `env:"MAX_PAGE_SIZE" envDefault:"100"`
	MaxUploadSize    int64         `env:"MAX_UPLOAD_SIZE" envDefault:"33554432"`
	MaxUploadMemory  int64         `env:"MAX_UPLOAD_MEMORY" envDefault:"33554432"`
	LogLevel         string        `env:"LOG_LEVEL" envDefault:"error"`
	// SDK will look for this header in the request, if it's not present, it will generate a random uuidv4
	RequestIDHeader string `env:"REQUEST_ID_HEADER" envDefault:"X-Request-ID"`
	// SDK will look for this header in the request, if it's not present, it will use request.RemoteAddr
	RealIPHeader string `env:"REAL_IP_HEADER" envDefault:"X-Real-IP"`
	// Session ID cookie key
	SidCookieKey        string `env:"SID_COOKIE_KEY" envDefault:"sid"`
	OauthStateCookieKey string `env:"OAUTH_STATE_COOKIE_KEY" envDefault:"oauthState"`

	TelegramBotToken string `env:"TELEGRAM_BOT_TOKEN"`

	// DEV-PLAN-019A: RLS enforcement mode (disabled/enforce).
	RLSEnforce string `env:"RLS_ENFORCE" envDefault:"disabled"`

	// Test endpoints - only enable in test environment
	EnableTestEndpoints bool `env:"ENABLE_TEST_ENDPOINTS" envDefault:"false"`

	// DEV-PLAN-024: Org auto-generated empty positions.
	EnableOrgAutoPositions bool `env:"ENABLE_ORG_AUTO_POSITIONS" envDefault:"true"`

	// DEV-PLAN-024: Enable extended assignment types (matrix/dotted).
	EnableOrgExtendedAssignmentTypes bool `env:"ENABLE_ORG_EXTENDED_ASSIGNMENT_TYPES" envDefault:"false"`

	// DEV-PLAN-027: Org rollout flags (tenant allowlist + rollback switches).
	OrgRolloutMode    string `env:"ORG_ROLLOUT_MODE" envDefault:"enabled"`
	OrgRolloutTenants string `env:"ORG_ROLLOUT_TENANTS" envDefault:""`
	OrgReadStrategy   string `env:"ORG_READ_STRATEGY" envDefault:"path"`
	OrgCacheEnabled   bool   `env:"ORG_CACHE_ENABLED" envDefault:"false"`

	// DEV-PLAN-029: Org deep-read derived tables (closure/snapshots) rollout flags.
	OrgDeepReadEnabled bool   `env:"ORG_DEEP_READ_ENABLED" envDefault:"false"`
	OrgDeepReadBackend string `env:"ORG_DEEP_READ_BACKEND" envDefault:"edges"`

	// DEV-PLAN-028: Org inheritance + role read side (default off).
	OrgInheritanceEnabled bool `env:"ORG_INHERITANCE_ENABLED" envDefault:"false"`
	OrgRoleReadEnabled    bool `env:"ORG_ROLE_READ_ENABLED" envDefault:"false"`

	// DEV-PLAN-030: Org change requests & preflight APIs.
	OrgChangeRequestsEnabled bool `env:"ORG_CHANGE_REQUESTS_ENABLED" envDefault:"false"`
	OrgPreflightEnabled      bool `env:"ORG_PREFLIGHT_ENABLED" envDefault:"false"`

	// DEV-PLAN-032: Org permission mapping & associations (default off).
	OrgSecurityGroupMappingsEnabled bool `env:"ORG_SECURITY_GROUP_MAPPINGS_ENABLED" envDefault:"false"`
	OrgLinksEnabled                 bool `env:"ORG_LINKS_ENABLED" envDefault:"false"`
	OrgPermissionPreviewEnabled     bool `env:"ORG_PERMISSION_PREVIEW_ENABLED" envDefault:"false"`

	// DEV-PLAN-031: Org data quality checks & fixes (CLI guard rails).
	OrgDataQualityEnabled   bool `env:"ORG_DATA_QUALITY_ENABLED" envDefault:"false"`
	OrgDataFixesMaxCommands int  `env:"ORG_DATA_FIXES_MAX_COMMANDS" envDefault:"100"`

	// Dev-only endpoints (e.g. /_dev/*). In production, these are disabled by default unless explicitly enabled.
	EnableDevEndpoints bool `env:"ENABLE_DEV_ENDPOINTS" envDefault:"false"`

	// GraphQL playground (/playground). In production, this is disabled by default unless explicitly enabled.
	EnableGraphQLPlayground bool `env:"ENABLE_GRAPHQL_PLAYGROUND" envDefault:"false"`

	// Ops endpoints guard (/health, /debug/prometheus). Enforced only in production.
	OpsGuardEnabled       bool   `env:"OPS_GUARD_ENABLED" envDefault:"true"`
	OpsGuardCIDRs         string `env:"OPS_GUARD_CIDRS" envDefault:""`
	OpsGuardToken         string `env:"OPS_GUARD_TOKEN" envDefault:""`
	OpsGuardBasicAuthUser string `env:"OPS_GUARD_BASIC_AUTH_USER" envDefault:""`
	OpsGuardBasicAuthPass string `env:"OPS_GUARD_BASIC_AUTH_PASS" envDefault:""`

	logFile *os.File
	logger  *logrus.Logger
}

func (c *Configuration) Logger() *logrus.Logger {
	return c.logger
}

func (c *Configuration) LogrusLogLevel() logrus.Level {
	switch c.LogLevel {
	case "silent":
		return logrus.PanicLevel
	case "error":
		return logrus.ErrorLevel
	case "warn":
		return logrus.WarnLevel
	case "info":
		return logrus.InfoLevel
	case "debug":
		return logrus.DebugLevel
	default:
		return logrus.ErrorLevel
	}
}

func (c *Configuration) Scheme() string {
	if c.GoAppEnvironment == Production { // assume 'https' on production mode
		return "https"
	}
	return "http"
}

func Use() *Configuration {
	return singleton()
}

func (c *Configuration) load(envFiles []string) error {
	n, err := LoadEnv(envFiles)
	if err != nil {
		return err
	}
	if n == 0 {
		wd, _ := os.Getwd()
		log.Println("No .env files found. Tried:")
		for _, file := range envFiles {
			log.Println(filepath.Join(wd, file))
		}
	}
	if err := env.Parse(c); err != nil {
		return err
	}

	// Validate rate limiting configuration
	if err := c.RateLimit.Validate(); err != nil {
		return fmt.Errorf("rate limit configuration error: %w", err)
	}

	if err := c.validateRLS(); err != nil {
		return err
	}
	if err := c.validateOrgRollout(); err != nil {
		return err
	}
	f, logger, err := logging.FileLogger(c.LogrusLogLevel(), c.Loki.LogPath)
	if err != nil {
		return err
	}
	c.logFile = f
	c.logger = logger

	c.Database.Opts = c.Database.ConnectionString()
	if c.GoAppEnvironment == Production {
		c.SocketAddress = fmt.Sprintf(":%d", c.ServerPort)
	} else {
		c.SocketAddress = fmt.Sprintf("localhost:%d", c.ServerPort)
	}

	// Update Domain and Origin dynamically if they weren't explicitly set via environment variables
	// This ensures logs show the correct port when PORT is set via environment
	if os.Getenv("DOMAIN") == "" {
		c.Domain = "localhost"
	}
	if os.Getenv("ORIGIN") == "" {
		// Only include port in Origin for development environment
		// Production and staging should use standard ports (80/443)
		if c.GoAppEnvironment == "development" {
			c.Origin = fmt.Sprintf("%s://%s:%d", c.Scheme(), c.Domain, c.ServerPort)
		} else {
			c.Origin = fmt.Sprintf("%s://%s", c.Scheme(), c.Domain)
		}
	}

	return nil
}

func (c *Configuration) validateRLS() error {
	mode := strings.ToLower(strings.TrimSpace(c.RLSEnforce))
	if mode == "" {
		mode = "disabled"
	}
	switch mode {
	case "disabled", "enforce":
	default:
		return fmt.Errorf("invalid RLS_ENFORCE=%q (expected disabled|enforce)", c.RLSEnforce)
	}

	if mode == "enforce" && strings.EqualFold(strings.TrimSpace(c.Database.User), "postgres") {
		return fmt.Errorf("RLS_ENFORCE=enforce requires a non-superuser DB_USER (postgres will bypass RLS)")
	}

	c.RLSEnforce = mode
	return nil
}

func (c *Configuration) validateOrgRollout() error {
	mode := strings.ToLower(strings.TrimSpace(c.OrgRolloutMode))
	if mode == "" {
		mode = "disabled"
	}
	switch mode {
	case "disabled", "enabled":
	default:
		return fmt.Errorf("invalid ORG_ROLLOUT_MODE=%q (expected disabled|enabled)", c.OrgRolloutMode)
	}
	c.OrgRolloutMode = mode

	strategy := strings.ToLower(strings.TrimSpace(c.OrgReadStrategy))
	if strategy == "" {
		strategy = "path"
	}
	switch strategy {
	case "path", "recursive":
	default:
		return fmt.Errorf("invalid ORG_READ_STRATEGY=%q (expected path|recursive)", c.OrgReadStrategy)
	}
	c.OrgReadStrategy = strategy

	deepReadBackend := strings.ToLower(strings.TrimSpace(c.OrgDeepReadBackend))
	if deepReadBackend == "" {
		deepReadBackend = "edges"
	}
	switch deepReadBackend {
	case "edges", "closure", "snapshot":
	default:
		return fmt.Errorf("invalid ORG_DEEP_READ_BACKEND=%q (expected edges|closure|snapshot)", c.OrgDeepReadBackend)
	}
	c.OrgDeepReadBackend = deepReadBackend

	rawTenants := strings.TrimSpace(c.OrgRolloutTenants)
	if rawTenants != "" {
		for _, part := range strings.FieldsFunc(rawTenants, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\n' || r == '\t'
		}) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if _, err := uuid.Parse(part); err != nil {
				return fmt.Errorf("invalid ORG_ROLLOUT_TENANTS entry=%q: %w", part, err)
			}
		}
	}

	return nil
}

// unload handles a graceful shutdown.
func (c *Configuration) Unload() {
	if c.logFile != nil {
		if err := c.logFile.Close(); err != nil {
			log.Printf("Failed to close log file: %v", err)
		}
	}
}
