package records

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/domain"
)

// Layout of an encrypted payload (SYNC.md § 3.5): a version byte, a nonce of
// 12 bytes, the content padded to a multiple of 256 bytes, a tag of 16 bytes.
const (
	payloadFormat   = 0x01
	payloadOverhead = 1 + 12 + 16
	payloadBlock    = 256
)

// Push applies the changes of a device, one by one, under the lock of the
// account (SYNC.md § 4.6). A change is applied when its baseVersion is the
// current version (or null for an unknown identifier); otherwise it is in
// conflict and nothing is written for it.
//
// The response is kept 24 hours under the Idempotency-Key of the device: the
// same key with the same body gets the same response without a new write, the
// same key with another body ErrIdempotencyMismatch. Since the key is checked
// under the lock, a request with the key of a request in progress waits for
// it, then gets its response.
func (c *RecordsApplication) Push(ctx context.Context, accountID, deviceID, idempotencyKey string, bodyHash []byte, changes []Change) (*PushOutput, error) {
	if len(changes) > c.Config.Sync.MaxChangesPerPush {
		return nil, domain.ErrTooManyChanges
	}
	idempotencyKey = strings.ToLower(idempotencyKey)

	var output *PushOutput
	var lastVersion int64
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		now := c.Now()
		replayed, err := c.replay(ctx, accountID, deviceID, idempotencyKey, bodyHash, now)
		if err != nil || replayed != nil {
			output = replayed
			return err
		}

		output = &PushOutput{Results: make([]PushResult, 0, len(changes))}
		applied := false
		for _, change := range changes {
			result, err := c.apply(ctx, account, change, now)
			if err != nil {
				return err
			}
			applied = applied || result.Status == StatusApplied
			output.Results = append(output.Results, result)
		}
		if applied {
			if err := c.Accounts.Update(ctx, account, "last_version", "usage_bytes"); err != nil {
				return err
			}
			lastVersion = account.LastVersion
		}

		response, err := json.Marshal(output)
		if err != nil {
			return err
		}
		return c.Idempotency.Save(ctx, &domain.IdempotencyKey{
			AccountID: accountID, DeviceID: deviceID, Key: idempotencyKey,
			BodyHash: bodyHash, Response: response, CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
	}

	if lastVersion > 0 {
		if c.Marker != nil {
			if err := c.Marker.Observe(lastVersion); err != nil {
				c.Logger.Error().Err(err).Str("component", "application.records.push").Str("event", "records.push.marker").
					Msg("Failed to update the epoch marker")
			}
		}
		if c.Notifier != nil {
			c.Notifier.Publish(accountID, deviceID, lastVersion)
		}
	}
	return output, nil
}

// replay returns the kept response of a key, or nil when the key is new or
// older than 24 hours.
func (c *RecordsApplication) replay(ctx context.Context, accountID, deviceID, key string, bodyHash []byte, now time.Time) (*PushOutput, error) {
	stored, err := c.Idempotency.Get(ctx, accountID, deviceID, key)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if now.Sub(stored.CreatedAt) >= IdempotencyRetention {
		return nil, nil
	}
	if !bytes.Equal(stored.BodyHash, bodyHash) {
		return nil, domain.ErrIdempotencyMismatch
	}
	var output PushOutput
	if err := json.Unmarshal(stored.Response, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

// apply checks and writes one change, in the transaction of ctx.
func (c *RecordsApplication) apply(ctx context.Context, account *domain.Account, change Change, now time.Time) (PushResult, error) {
	id := strings.ToLower(change.ID)
	result := PushResult{ID: id}
	reject := func(code string) (PushResult, error) {
		result.Status = StatusRejected
		result.Code = code
		return result, nil
	}

	payload, err := base64.StdEncoding.DecodeString(change.Payload)
	if err != nil {
		return reject(CodeInvalidPayload)
	}
	if len(payload) > c.Config.Sync.MaxPayloadBytes {
		return reject(CodePayloadTooLarge)
	}
	if !validPayload(payload) || change.KeyVersion != account.KeyVersion {
		return reject(CodeInvalidPayload)
	}

	current, err := c.Records.Get(ctx, account.ID, id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		current = nil
	case err != nil:
		return result, err
	}
	switch {
	case current == nil && change.BaseVersion != nil:
		// Unknown identifier (never written, or deletion purged): the client
		// sends it again as a creation.
		result.Status = StatusConflict
		return result, nil
	case current != nil && (change.BaseVersion == nil || *change.BaseVersion != current.Version):
		result.Status = StatusConflict
		result.Current = recordFromDomain(current)
		return result, nil
	}

	previousSize := 0
	if current != nil {
		previousSize = current.Size
	}
	usage := account.UsageBytes - int64(previousSize) + int64(len(payload))
	// A deletion frees space, it is never refused for the quota.
	if quota := c.Config.Sync.MaxAccountBytes; quota > 0 && !change.Deleted && usage > int64(quota) {
		return reject(CodeQuotaExceeded)
	}

	version := account.NextVersion(now)
	record := &domain.Record{
		AccountID:  account.ID,
		ID:         id,
		Version:    version,
		Deleted:    change.Deleted,
		KeyVersion: change.KeyVersion,
		Payload:    payload,
		Size:       len(payload),
		UpdatedAt:  now,
	}
	if err := c.Records.Put(ctx, record); err != nil {
		return result, err
	}
	account.UsageBytes = usage
	result.Status = StatusApplied
	result.Version = &version
	return result, nil
}

// validPayload checks the layout of an encrypted payload, without opening it.
func validPayload(payload []byte) bool {
	return len(payload) >= payloadOverhead+payloadBlock &&
		payload[0] == payloadFormat &&
		(len(payload)-payloadOverhead)%payloadBlock == 0
}
