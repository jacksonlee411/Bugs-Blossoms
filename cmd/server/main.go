package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	internalassets "github.com/iota-uz/iota-sdk/internal/assets"
	"github.com/iota-uz/iota-sdk/internal/server"
	"github.com/iota-uz/iota-sdk/modules"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers"
	orgoutboxdispatcher "github.com/iota-uz/iota-sdk/modules/org/infrastructure/outbox"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
	"github.com/iota-uz/iota-sdk/pkg/logging"
	"github.com/iota-uz/iota-sdk/pkg/metrics"
	"github.com/iota-uz/iota-sdk/pkg/outbox"
	eventbusdispatcher "github.com/iota-uz/iota-sdk/pkg/outbox/dispatchers/eventbus"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			configuration.Use().Unload()
			log.Println(r)
			debug.PrintStack()
			os.Exit(1)
		}
	}()

	conf := configuration.Use()
	logger := conf.Logger()

	// Set up OpenTelemetry if enabled
	var tracingCleanup func()
	if conf.OpenTelemetry.Enabled {
		tracingCleanup = logging.SetupTracing(
			context.Background(),
			conf.OpenTelemetry.ServiceName,
			conf.OpenTelemetry.TempoURL,
		)
		defer tracingCleanup()
		logger.Info("OpenTelemetry tracing enabled, exporting to Tempo at " + conf.OpenTelemetry.TempoURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	pool, err := pgxpool.New(ctx, conf.Database.Opts)
	if err != nil {
		panic(err)
	}
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
			UserRepository: persistence.NewUserRepository(persistence.NewUploadRepository()),
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}),
	})
	if err := modules.Load(app, modules.BuiltInModules...); err != nil {
		log.Fatalf("failed to load modules: %v", err)
	}

	startOutboxBackground(conf, pool, logger, app.EventPublisher())

	app.RegisterNavItems(modules.NavLinks...)
	app.RegisterHashFsAssets(internalassets.HashFS)
	app.RegisterControllers(
		controllers.NewStaticFilesController(app.HashFsAssets()),
		controllers.NewGraphQLController(app),
	)
	if conf.Prometheus.Enabled {
		app.RegisterControllers(metrics.NewPrometheusController(conf.Prometheus.Path))
	}
	options := &server.DefaultOptions{
		Logger:        logger,
		Configuration: conf,
		Application:   app,
		Pool:          pool,
		Entrypoint:    "server",
	}
	serverInstance, err := server.Default(options)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	log.Printf("Listening on: %s\n", conf.Origin)
	if err := serverInstance.Start(conf.SocketAddress); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func startOutboxBackground(
	conf *configuration.Configuration,
	pool *pgxpool.Pool,
	logger *logrus.Logger,
	bus eventbus.EventBus,
) {
	outboxLog := logger.WithField("component", "outbox")

	relayTables, relayTablesErr := outbox.ParseIdentifierList(conf.Outbox.RelayTables)
	if relayTablesErr != nil {
		outboxLog.WithError(relayTablesErr).Warn("outbox: invalid OUTBOX_RELAY_TABLES; relay disabled")
		relayTables = nil
	}

	var cleanerTables []pgx.Identifier
	if conf.Outbox.CleanerTables == "" {
		cleanerTables = relayTables
	} else {
		var cleanerTablesErr error
		cleanerTables, cleanerTablesErr = outbox.ParseIdentifierList(conf.Outbox.CleanerTables)
		if cleanerTablesErr != nil {
			outboxLog.WithError(cleanerTablesErr).Warn("outbox: invalid OUTBOX_CLEANER_TABLES; cleaner disabled")
			cleanerTables = nil
		}
	}

	if conf.Outbox.RelayEnabled {
		if len(relayTables) == 0 {
			if relayTablesErr == nil {
				outboxLog.Info("outbox: relay enabled but OUTBOX_RELAY_TABLES is empty")
			}
		} else {
			eb, ok := bus.(eventbus.EventBusWithError)
			if !ok {
				outboxLog.Warn("outbox: eventbus does not support PublishE; relay not started")
			} else {
				dispatcher := eventbusdispatcher.New(eb)
				for _, table := range relayTables {
					var relayDispatcher outbox.Dispatcher = dispatcher
					if outbox.TableLabel(table) == "public.org_outbox" {
						relayDispatcher = orgoutboxdispatcher.NewDispatcher(eb)
					}
					relay, err := outbox.NewRelay(pool, table, relayDispatcher, outbox.RelayOptions{
						PollInterval:    conf.Outbox.RelayPollInterval,
						BatchSize:       conf.Outbox.RelayBatchSize,
						LockTTL:         conf.Outbox.RelayLockTTL,
						MaxAttempts:     conf.Outbox.RelayMaxAttempts,
						SingleActive:    conf.Outbox.RelaySingleActive,
						LastErrorMaxLen: conf.Outbox.LastErrorMaxBytes,
						DispatchTimeout: conf.Outbox.RelayDispatchTimeout,
						Logger:          outboxLog.WithField("table", outbox.TableLabel(table)),
					})
					if err != nil {
						outboxLog.WithError(err).Warn("outbox: failed to create relay")
						continue
					}
					go func(r *outbox.Relay) {
						if err := r.Run(context.Background()); err != nil {
							outboxLog.WithError(err).Error("outbox: relay stopped")
						}
					}(relay)
				}
			}
		}
	}

	if conf.Outbox.CleanerEnabled && len(cleanerTables) > 0 {
		for _, table := range cleanerTables {
			cleaner, err := outbox.NewCleaner(pool, table, outbox.CleanerOptions{
				Enabled:               true,
				Interval:              conf.Outbox.CleanerInterval,
				Retention:             conf.Outbox.CleanerRetention,
				DeadRetention:         conf.Outbox.CleanerDeadRetention,
				DeadAttemptsThreshold: conf.Outbox.RelayMaxAttempts,
				Logger:                outboxLog.WithField("table", outbox.TableLabel(table)),
			})
			if err != nil {
				outboxLog.WithError(err).Warn("outbox: failed to create cleaner")
				continue
			}
			go func(c *outbox.Cleaner) {
				if err := c.Run(context.Background()); err != nil {
					outboxLog.WithError(err).Error("outbox: cleaner stopped")
				}
			}(cleaner)
		}
	} else if conf.Outbox.CleanerEnabled && len(cleanerTables) == 0 {
		outboxLog.Info("outbox: cleaner enabled but no tables configured")
	}
}
