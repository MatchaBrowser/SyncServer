package records

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"
)

// Changes returns the records of the account written after the cursor, by
// increasing version. Without a cursor, the reading starts from the
// beginning. A cursor of another epoch (restored backup), or older than a
// purged deletion, is expired (SYNC.md § 4.6, § 4.9).
func (c *RecordsApplication) Changes(ctx context.Context, accountID, since string, limit int) (*ChangesOutput, error) {
	switch {
	case limit <= 0:
		limit = DefaultChangesLimit
	case limit > MaxChangesLimit:
		limit = MaxChangesLimit
	}

	state, err := c.ServerState.Get(ctx)
	if err != nil {
		return nil, err
	}
	account, err := c.Accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	cursor := Cursor{Epoch: state.Epoch}
	if since != "" {
		received, err := DecodeCursor(since)
		if err != nil {
			return nil, err
		}
		if received.Epoch != state.Epoch || received.Version < account.MaxPurgedVersion {
			return nil, domain.ErrCursorExpired
		}
		cursor.Version = received.Version
	}

	records, err := c.Records.ChangesSince(ctx, accountID, cursor.Version, limit+1)
	if err != nil {
		return nil, err
	}
	output := &ChangesOutput{Changes: make([]Record, 0, min(len(records), limit))}
	if len(records) > limit {
		records = records[:limit]
		output.HasMore = true
	}
	for i := range records {
		output.Changes = append(output.Changes, *recordFromDomain(&records[i]))
		cursor.Version = records[i].Version
	}
	output.Cursor = cursor.Encode()
	return output, nil
}
