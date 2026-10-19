package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/matchabrowser/syncserver/application/auth"
	"github.com/matchabrowser/syncserver/application/session"
	"github.com/matchabrowser/syncserver/application/user"
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/infrastructure/jobs"
	"github.com/matchabrowser/syncserver/infrastructure/logger"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	routes "github.com/matchabrowser/syncserver/interfaces/http"

	"github.com/gofiber/fiber/v3"
	"github.com/urfave/cli/v3"
)

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
			return runServer(*cfg)
		},
	}
}

// getFlags returns the list of CLI flags required for the server command.
func getFlags(cfg *config.Config) (list []cli.Flag) {
	list = append(list, config.GenericFlags(cfg)...)
	list = append(list, config.ServerFlags(cfg)...)
	list = append(list, config.LoggerFlags(cfg)...)
	list = append(list, config.DatabaseFlags(cfg)...)
	list = append(list, config.SessionFlags(cfg)...)
	list = append(list, config.RegistrationFlags(cfg)...)
	return
}

// runServer initializes the necessary dependencies and starts the HTTP server.
func runServer(cfg config.Config) error {
	var err error

	// Initialize dependencies
	deps := infrastructure.Deps{
		Config: cfg,
	}

	// Initialize logger
	deps.Logger = logger.NewLogger(cfg.Logger.Level, cfg.Logger.Pretty, cfg.Version)
	logger := deps.Logger.With().Str("component", "interfaces.cli.http.runserver").Logger()

	// Validate JWT secret key
	if len(cfg.Session.SecretKey) < 32 {
		logger.Fatal().Msg("SESSION_SECRET_KEY must be set and at least 32 characters long")
		return fmt.Errorf("SESSION_SECRET_KEY must be at least 32 characters")
	}

	// Initialize other cron scheduler (go-cron)
	deps.CronScheduler, err = cronscheduler.Configure(deps.Logger)
	if err != nil {
		logger.Fatal().Err(err).Str("event", "http.runserver.cronscheduler.configure").Msg("Failed to configure cron scheduler")
		return err
	}

	// Initialize database connection (gorm)
	deps.Database, err = database.Configure(deps.Config, deps.Logger)
	if err != nil {
		logger.Fatal().Err(err).Str("event", "http.runserver.database.configure").Msg("Failed to configure database connection")
		return err
	}

	// Initialize application services
	userPers := persistence.NewUserPers(deps.Database.Db)
	sessionPers := persistence.NewSessionPers(deps.Database.Db)

	deps.UserApplication = user.NewUserApplication(deps.Config, deps.Logger, userPers)
	deps.SessionApplication = session.NewSessionApplication(deps.Config, deps.Logger, sessionPers)
	deps.AuthApplication = auth.NewAuthApplication(deps.Config, deps.Logger)

	// Inject port dependencies (after construction to avoid circular dependencies)
	deps.AuthApplication.UserApplication = deps.UserApplication
	deps.AuthApplication.SessionApplication = deps.SessionApplication
	deps.SessionApplication.UserApplication = deps.UserApplication

	// Initialize HTTP server (fiber + fiberoapi)
	deps.Http, err = http.Configure(deps.Config, deps.Logger, deps.SessionApplication, true)
	if err != nil {
		logger.Fatal().Err(err).Str("event", "http.runserver.http.configure").Msg("Failed to configure HTTP server")
		return err
	}

	// Setup cron jobs
	configJobs := jobs.Config{
		Logger:        deps.Logger,
		CronScheduler: deps.CronScheduler,
		SessionApp:    *deps.SessionApplication,
	}

	err = configJobs.SetupJobs()
	if err != nil {
		logger.Fatal().Err(err).Str("event", "http.runserver.jobs.setup").Msg("Failed to setup cron jobs")
		return err
	}

	// Setup routes
	routes.SetupRoutes(deps)

	// Start HTTP server
	logger.Info().Str("event", "http.runserver.http.listen").Msgf("Starting HTTP server on port %d", cfg.Server.Port)
	err = deps.Http.Fiber.Listen(":"+strconv.Itoa(cfg.Server.Port), fiber.ListenConfig{
		DisableStartupMessage: true,
	})
	if err != nil {
		logger.Fatal().Err(err).Str("event", "http.runserver.http.listen").Msg("Failed to start HTTP server")
		return err
	}

	return nil
}
