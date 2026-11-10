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
// An atomic push applies every change or none (SYNC.md § 4.6, VAULT.md
// § 5.1): when one of them is in conflict or rejected, nothing is written,
// and the others are aborted. The same record twice is ErrDuplicateChange.
//
// The response is kept 24 hours under the Idempotency-Key of the device: the
// same key with the same body gets the same response without a new write, the
// same key with another body ErrIdempotencyMismatch. Since the key is checked
// under the lock, a request with the key of a request in progress waits for
// it, then gets its response.
func (c *RecordsApplication) Push(ctx context.Context, accountID, deviceID, idempotencyKey string, bodyHash []byte, changes []Change, atomic bool) (*PushOutput, error) {
	if len(changes) > c.Config.Sync.MaxChangesPerPush {
		return nil, domain.ErrTooManyChanges
	}
	if atomic && hasDuplicates(changes) {
		return nil, domain.ErrDuplicateChange
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

		var applied bool
		if atomic {
			output, applied, err = c.applyAll(ctx, account, changes, now)
		} else {
			output, applied, err = c.applyEach(ctx, account, changes, now)
		}
		if err != nil {
			return err
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

// applyEach checks and writes the changes one by one: each is applied, in
// conflict or rejected on its own.
func (c *RecordsApplication) applyEach(ctx context.Context, account *domain.Account, changes []Change, now time.Time) (*PushOutput, bool, error) {
	output := &PushOutput{Results: make([]PushResult, 0, len(changes))}
	applied := false
	for _, change := range changes {
		result, write, err := c.check(ctx, account, change, account.UsageBytes, true)
		if err != nil {
			return nil, false, err
		}
		if write != nil {
			if result, err = c.write(ctx, account, write, now); err != nil {
				return nil, false, err
			}
			applied = true
		}
		output.Results = append(output.Results, result)
	}
	return output, applied, nil
}

// applyAll checks every change first, the usage of the account growing with
// each, then writes them all, or none if one would not be applied. The quota
// counts the balance of the whole push: it is refused only when the push adds
// space and the usage then exceeds the quota, and only the changes that add
// space are rejected for it.
func (c *RecordsApplication) applyAll(ctx context.Context, account *domain.Account, changes []Change, now time.Time) (*PushOutput, bool, error) {
	output := &PushOutput{Results: make([]PushResult, len(changes))}
	writes := make([]*pendingWrite, len(changes))
	usage := account.UsageBytes
	complete := true
	for i, change := range changes {
		result, write, err := c.check(ctx, account, change, usage, false)
		if err != nil {
			return nil, false, err
		}
		output.Results[i] = result
		writes[i] = write
		if write == nil {
			complete = false
			continue
		}
		usage = write.usage
	}
	if quota := c.Config.Sync.MaxAccountBytes; quota > 0 && usage > account.UsageBytes && usage > int64(quota) {
		// A deletion frees space: it is never refused for the quota.
		for i, write := range writes {
			if write != nil && write.grows && !write.record.Deleted {
				output.Results[i] = PushResult{ID: write.record.ID, Status: StatusRejected, Code: CodeQuotaExceeded}
				writes[i] = nil
				complete = false
			}
		}
	}
	if !complete {
		for i, write := range writes {
			if write != nil {
				output.Results[i] = PushResult{ID: write.record.ID, Status: StatusAborted}
			}
		}
		return output, false, nil
	}
	for i, write := range writes {
		result, err := c.write(ctx, account, write, now)
		if err != nil {
			return nil, false, err
		}
		output.Results[i] = result
	}
	return output, len(writes) > 0, nil
}

// pendingWrite is a change that check accepted, before write gives it a
// version.
type pendingWrite struct {
	record *domain.Record
	// usage is the usage of the account once the change is written.
	usage int64
	// grows: the change takes more space than the record it replaces.
	grows bool
}

// check tells whether a change would be applied, given the usage of the
// account, without writing anything. It returns the write to do, or nil
// with the result of a conflict or of a rejection. checkQuota refuses a
// change over the quota on its own (an ordinary push).
func (c *RecordsApplication) check(ctx context.Context, account *domain.Account, change Change, usage int64, checkQuota bool) (PushResult, *pendingWrite, error) {
	id := strings.ToLower(change.ID)
	result := PushResult{ID: id}
	reject := func(code string) (PushResult, *pendingWrite, error) {
		result.Status = StatusRejected
		result.Code = code
		return result, nil, nil
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
		return result, nil, err
	}
	switch {
	case current == nil && change.BaseVersion != nil:
		// Unknown identifier (never written, or deletion purged): the client
		// sends it again as a creation.
		result.Status = StatusConflict
		return result, nil, nil
	case current != nil && (change.BaseVersion == nil || *change.BaseVersion != current.Version):
		result.Status = StatusConflict
		result.Current = recordFromDomain(current)
		return result, nil, nil
	}

	previousSize := 0
	if current != nil {
		previousSize = current.Size
	}
	usage = usage - int64(previousSize) + int64(len(payload))
	// A deletion frees space, it is never refused for the quota.
	if quota := c.Config.Sync.MaxAccountBytes; checkQuota && quota > 0 && !change.Deleted && usage > int64(quota) {
		return reject(CodeQuotaExceeded)
	}
	record := &domain.Record{
		AccountID:  account.ID,
		ID:         id,
		Deleted:    change.Deleted,
		KeyVersion: change.KeyVersion,
		Payload:    payload,
		Size:       len(payload),
	}
	return result, &pendingWrite{record: record, usage: usage, grows: len(payload) > previousSize}, nil
}

// write gives a checked change its version and writes it, in the transaction
// of ctx.
func (c *RecordsApplication) write(ctx context.Context, account *domain.Account, write *pendingWrite, now time.Time) (PushResult, error) {
	version := account.NextVersion(now)
	write.record.Version = version
	write.record.UpdatedAt = now
	if err := c.Records.Put(ctx, write.record); err != nil {
		return PushResult{}, err
	}
	account.UsageBytes = write.usage
	return PushResult{ID: write.record.ID, Status: StatusApplied, Version: &version}, nil
}

// hasDuplicates tells whether two changes name the same record.
func hasDuplicates(changes []Change) bool {
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		id := strings.ToLower(change.ID)
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

// validPayload checks the layout of an encrypted payload, without opening it.
func validPayload(payload []byte) bool {
	return len(payload) >= payloadOverhead+payloadBlock &&
		payload[0] == payloadFormat &&
		(len(payload)-payloadOverhead)%payloadBlock == 0
}
