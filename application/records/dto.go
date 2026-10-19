package records

import (
	"encoding/json"

	"github.com/matchabrowser/syncserver/domain"
)

// These types are also the JSON of the API: the response of a push is kept
// as it was sent, for the Idempotency-Key (SYNC.md § 4.6).

// Record is a record as the server returns it.
type Record struct {
	ID         string           `json:"id"`
	Version    int64            `json:"version"`
	Deleted    bool             `json:"deleted"`
	KeyVersion int              `json:"keyVersion"`
	Payload    []byte           `json:"payload"`
	UpdatedAt  domain.Timestamp `json:"updatedAt"`
}

func recordFromDomain(r *domain.Record) *Record {
	return &Record{ID: r.ID, Version: r.Version, Deleted: r.Deleted, KeyVersion: r.KeyVersion, Payload: r.Payload, UpdatedAt: domain.Timestamp(r.UpdatedAt)}
}

type ChangesOutput struct {
	Changes []Record `json:"changes"`
	Cursor  string   `json:"cursor"`
	HasMore bool     `json:"hasMore"`
}

// Change is a change pushed by a device. The payload is still in base 64: a
// malformed one rejects this change only.
type Change struct {
	ID          string
	BaseVersion *int64
	Deleted     bool
	KeyVersion  int
	Payload     string
}

// Status of a pushed change.
const (
	StatusApplied  = "applied"
	StatusConflict = "conflict"
	StatusRejected = "rejected"
)

// Codes of a rejected change (SYNC.md § 4.6).
const (
	CodePayloadTooLarge = "payload_too_large"
	CodeQuotaExceeded   = "quota_exceeded"
	CodeInvalidPayload  = "invalid_payload"
)

type PushResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Version *int64 `json:"version,omitempty"`
	// Current is the record on the server, for a conflict: null when the
	// server does not know the identifier.
	Current *Record `json:"current,omitempty"`
	Code    string  `json:"code,omitempty"`
}

// MarshalJSON writes "current": null for a conflict on an unknown record.
func (r PushResult) MarshalJSON() ([]byte, error) {
	type plain PushResult
	if r.Status != StatusConflict {
		return json.Marshal(plain(r))
	}
	return json.Marshal(struct {
		plain
		Current *Record `json:"current"`
	}{plain(r), r.Current})
}

type PushOutput struct {
	Results []PushResult `json:"results"`
}
