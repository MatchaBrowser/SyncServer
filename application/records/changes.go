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

	// The sizes first, so that the payloads left out of the page are never
	// loaded. A record rewritten in between only moves to a later version: the
	// page then holds the next record instead, a little over the bound at worst.
	sizes, err := c.Records.SizesSince(ctx, accountID, cursor.Version, limit+1)
	if err != nil {
		return nil, err
	}
	count := pageLength(sizes, limit)
	output := &ChangesOutput{Changes: make([]Record, 0, count), HasMore: count < len(sizes)}
	if count > 0 {
		records, err := c.Records.ChangesSince(ctx, accountID, cursor.Version, count)
		if err != nil {
			return nil, err
		}
		for i := range records {
			output.Changes = append(output.Changes, *recordFromDomain(&records[i]))
			cursor.Version = records[i].Version
		}
	}
	output.Cursor = cursor.Encode()
	return output, nil
}

// pageLength is how many records, given by their payload sizes, make a page:
// at most limit, and no more than MaxChangesBytes, except for a first record
// larger than that, which comes alone.
func pageLength(sizes []int, limit int) int {
	total := 0
	for i, size := range sizes {
		total += size
		if i == limit || (i > 0 && total > MaxChangesBytes) {
			return i
		}
	}
	return len(sizes)
}
