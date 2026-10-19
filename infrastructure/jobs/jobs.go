package jobs

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/blobs"
	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog"
)

// purgeEvery is the period of the purge job.
const purgeEvery = 15 * time.Minute

// Config holds what the periodic jobs need.
type Config struct {
	Logger             zerolog.Logger
	CronScheduler      cronscheduler.Config
	AccountApplication *account.AccountApplication
	RecordsApplication *records.RecordsApplication
	BlobsApplication   *blobs.BlobsApplication
}

func (c *Config) SetupJobs() error {
	logger := c.Logger.With().Str("component", "infrastructure.jobs").Logger()

	if err := c.Purge(); err != nil {
		logger.Error().Err(err).Msg("failed to setup Purge job")
		return err
	}

	return nil
}

// Purge removes what has expired: tokens, traces of deleted accounts,
// invitations, responses kept for the Idempotency-Key, deletions older than
// 180 days (SYNC.md § 4.9), and temporary files of interrupted uploads.
func (c *Config) Purge() error {
	logger := c.Logger.With().Str("component", "infrastructure.jobs.purge").Logger()
	_, err := c.CronScheduler.CronScheduler.NewJob(
		gocron.DurationJob(purgeEvery),
		gocron.NewTask(func() {
			ctx, cancel := context.WithTimeout(context.Background(), purgeEvery)
			defer cancel()
			if err := c.AccountApplication.PurgeExpired(ctx); err != nil {
				logger.Error().Err(err).Str("event", "jobs.purge.accounts").Msg("Failed to purge expired tokens and invitations")
			}
			if err := c.RecordsApplication.Purge(ctx); err != nil {
				logger.Error().Err(err).Str("event", "jobs.purge.records").Msg("Failed to purge old deletions")
			}
			if err := c.BlobsApplication.PurgeTemp(); err != nil {
				logger.Error().Err(err).Str("event", "jobs.purge.blobs").Msg("Failed to purge temporary package files")
			}
		}),
		gocron.WithName("Purge"),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	return err
}
