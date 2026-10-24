package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/epoch"
	"github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/infrastructure/jobs"
	"github.com/matchabrowser/syncserver/infrastructure/logger"
	"github.com/matchabrowser/syncserver/infrastructure/migration"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	routes "github.com/matchabrowser/syncserver/interfaces/http"

	"github.com/gofiber/fiber/v3"
	"github.com/urfave/cli/v3"
)

// shutdownTimeout bounds the time given to the requests in flight when the
// server stops.
const shutdownTimeout = 10 * time.Second

// NewInstance creates a new CLI command for starting the server.
// It's called by the main application to add the "server" command to the CLI.
func NewInstance(version string) *cli.Command {
	cfg := &config.Config{}
	cfg.Version = version
	serverFlags := getFlags(cfg)

	return &cli.Command{
		Name:  "server",
		Usage: "Start the SyncServer HTTP server",
		Flags: serverFlags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runServer(ctx, *cfg)
		},
	}
}

// getFlags returns the list of CLI flags required for the server command.
func getFlags(cfg *config.Config) (list []cli.Flag) {
	list = append(list, config.GenericFlags(cfg)...)
	list = append(list, config.ServerFlags(cfg)...)
	list = append(list, config.LoggerFlags(cfg)...)
	list = append(list, config.DatabaseFlags(cfg)...)
	list = append(list, config.RegistrationFlags(cfg)...)
	list = append(list, config.SyncFlags(cfg)...)
	list = append(list, config.KdfFlags(cfg)...)
	list = append(list, config.Argon2Flags(cfg)...)
	list = append(list, config.StorageFlags(cfg)...)
	return
}

// runServer initializes the necessary dependencies and starts the HTTP
// server, until SIGINT or SIGTERM.
func runServer(ctx context.Context, cfg config.Config) error {
	var err error

	// Initialize dependencies
	deps := infrastructure.Deps{
		Config: cfg,
	}

	// Initialize logger
	deps.Logger = logger.NewLogger(cfg.Logger.Level, cfg.Logger.Pretty, cfg.Version)
	logger := deps.Logger.With().Str("component", "interfaces.cli.http.runserver").Logger()

	if err := cfg.Validate(); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.config.validate").Msg("Invalid configuration")
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if err := os.MkdirAll(cfg.Storage.DataDir, 0o700); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.storage.mkdir").Msg("Failed to create the data directory")
		return err
	}

	// Initialize database connection (gorm)
	deps.Database, err = database.Configure(deps.Config, deps.Logger)
	if err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.database.configure").Msg("Failed to configure database connection")
		return err
	}
	defer func() {
		if err := deps.Database.Close(); err != nil {
			logger.Error().Err(err).Str("event", "http.runserver.database.close").Msg("Failed to close the database")
		}
	}()

	// The migrations are applied by the migration command: the server only
	// refuses an outdated schema.
	if err := migration.CheckUpToDate(deps.Logger, deps.Database.Db); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.migration.check").Msg("Database schema not up to date")
		return err
	}

	// Detect a restored backup: a new epoch expires the cursors of the clients.
	deps.EpochMarker, err = epoch.Check(ctx, deps.Logger, filepath.Join(cfg.Storage.DataDir, epoch.FileName), persistence.NewServerStatePers(deps.Database.Db))
	if err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.epoch.check").Msg("Failed to check the epoch marker")
		return err
	}

	// Initialize applications
	if err := wireApplications(&deps); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.applications").Msg("Failed to set up the applications")
		return err
	}

	// Initialize cron scheduler (go-cron)
	deps.CronScheduler, err = cronscheduler.Configure(deps.Logger)
	if err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.cronscheduler.configure").Msg("Failed to configure cron scheduler")
		return err
	}
	defer func() {
		if err := deps.CronScheduler.CronScheduler.Shutdown(); err != nil {
			logger.Error().Err(err).Str("event", "http.runserver.cronscheduler.shutdown").Msg("Failed to stop the cron scheduler")
		}
	}()

	// Initialize HTTP server (fiber + fiberoapi)
	deps.Http, err = http.Configure(deps.Config, deps.Logger, routes.HTTPOptions(deps))
	if err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.http.configure").Msg("Failed to configure HTTP server")
		return err
	}

	// Setup cron jobs
	configJobs := jobs.Config{
		Logger:             deps.Logger,
		CronScheduler:      deps.CronScheduler,
		AccountApplication: deps.AccountApplication,
		RecordsApplication: deps.RecordsApplication,
		BlobsApplication:   deps.BlobsApplication,
	}
	if err := configJobs.SetupJobs(); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.jobs.setup").Msg("Failed to setup cron jobs")
		return err
	}

	// Setup routes
	if err := routes.SetupRoutes(deps); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.routes.setup").Msg("Failed to setup routes")
		return err
	}

	// Start HTTP server, and stop it on SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenErr := make(chan error, 1)
	go func() {
		logger.Info().Str("event", "http.runserver.http.listen").Msgf("Starting HTTP server on port %d", cfg.Server.Port)
		listenErr <- deps.Http.Fiber.Listen(":"+strconv.Itoa(cfg.Server.Port), fiber.ListenConfig{
			DisableStartupMessage: true,
		})
	}()

	select {
	case err := <-listenErr:
		if err != nil {
			logger.Error().Err(err).Str("event", "http.runserver.http.listen").Msg("Failed to start HTTP server")
		}
		return err
	case <-ctx.Done():
	}

	logger.Info().Str("event", "http.runserver.shutdown").Msg("Stopping the server")
	// The event streams never end on their own: close them first, so that
	// the shutdown does not wait for them.
	deps.EventsHub.CloseAll()
	if err := deps.Http.Fiber.ShutdownWithTimeout(shutdownTimeout); err != nil {
		logger.Error().Err(err).Str("event", "http.runserver.http.shutdown").Msg("Failed to stop the HTTP server")
		return err
	}
	if err := <-listenErr; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
