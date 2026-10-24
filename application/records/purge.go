package records

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"
)

// Purge removes the deletions older than 180 days, and the responses kept for
// the Idempotency-Key older than 24 hours. A purged deletion raises the
// max_purged_version of its account: a cursor older than it is expired, and
// the client reads everything again (SYNC.md § 4.9).
func (c *RecordsApplication) Purge(ctx context.Context) error {
	logger := c.Logger.With().Str("component", "application.records.purge").Logger()
	now := c.Now()

	cutoff := now.Add(-DeletionRetention)
	accounts, err := c.Records.AccountsWithDeletionsBefore(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, accountID := range accounts {
		err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
			purged, err := c.Records.PurgeDeletions(ctx, accountID, cutoff)
			if err != nil || purged.Count == 0 {
				return err
			}
			account.MaxPurgedVersion = max(account.MaxPurgedVersion, purged.MaxVersion)
			account.UsageBytes = max(0, account.UsageBytes-purged.Bytes)
			logger.Info().Str("event", "records.purge.deletions").Str("account_id", accountID).Int64("count", purged.Count).
				Msg("Old deletions purged")
			return c.Accounts.Update(ctx, account, "max_purged_version", "usage_bytes")
		})
		if err != nil {
			return err
		}
	}

	_, err = c.Idempotency.PurgeBefore(ctx, now.Add(-IdempotencyRetention))
	return err
}
