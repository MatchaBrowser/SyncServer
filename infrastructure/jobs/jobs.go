package jobs

import (
	"github.com/matchabrowser/syncserver/application/session"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/rs/zerolog"
)

type Config struct {
	Logger        zerolog.Logger
	CronScheduler cronscheduler.Config
	SessionApp    session.SessionApp
}

func (c *Config) SetupJobs() error {
	logger := c.Logger.With().Str("component", "infrastructure.jobs").Logger()

	if err := c.CleanUsersSessions(); err != nil {
		logger.Error().Err(err).Msg("failed to setup CleanUsersSessions job")
		return err
	}

	return nil
}
